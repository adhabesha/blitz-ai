package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

//go:embed templates/*.html static/*
var assets embed.FS

const (
	dailyEvals = 20 // evaluations per user per day
	dailyAI    = 6  // AI-assisted evaluations per user per day (protects the free Gemini quota)
)

var errRate = errors.New("rate limited")

type server struct {
	auth        *auth.Client
	db          *firestore.Client
	tpl         *template.Template
	cfg         map[string]string
	gkey, model string
}

func main() {
	ctx := context.Background()
	opt := option.WithCredentialsJSON([]byte(os.Getenv("FIREBASE_CREDENTIALS")))
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: os.Getenv("FIREBASE_PROJECT_ID")}, opt)
	if err != nil {
		log.Fatal(err)
	}
	a, err := app.Auth(ctx)
	if err != nil {
		log.Fatal(err)
	}
	db, err := app.Firestore(ctx)
	if err != nil {
		log.Fatal(err)
	}
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	s := &server{
		auth: a, db: db, gkey: os.Getenv("GEMINI_API_KEY"), model: model,
		tpl: template.Must(template.ParseFS(assets, "templates/*.html")),
		cfg: map[string]string{
			"APIKey":     os.Getenv("FIREBASE_WEB_API_KEY"),
			"AuthDomain": os.Getenv("FIREBASE_AUTH_DOMAIN"),
			"ProjectID":  os.Getenv("FIREBASE_PROJECT_ID"),
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.FS(assets)))
	mux.HandleFunc("/", s.page("landing.html"))
	mux.HandleFunc("/dashboard", s.page("dashboard.html"))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/api/evaluate", s.evaluate)
	mux.HandleFunc("/api/reports", s.reports)
	mux.HandleFunc("/auth/google/callback", s.googleCallback)
	mux.HandleFunc("/api/google/start", s.googleStart)
	mux.HandleFunc("/api/google/status", s.googleStatus)
	mux.HandleFunc("/api/google/assets", s.googleAssets)
	mux.HandleFunc("/api/google/token", s.googleToken)
	mux.HandleFunc("/api/google/disconnect", s.googleDisconnect)
	mux.HandleFunc("/api/optimize", s.optimize)
	mux.HandleFunc("/api/youtube/apply", s.applyYouTube)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("listening on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func (s *server) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if name == "landing.html" && r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if err := s.tpl.ExecuteTemplate(w, name, s.cfg); err != nil {
			log.Println(err)
		}
	}
}

func (s *server) uid(r *http.Request) (string, error) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	t, err := s.auth.VerifyIDToken(r.Context(), tok)
	if err != nil {
		return "", err
	}
	return t.UID, nil
}

func (s *server) reports(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	col := s.db.Collection("users").Doc(uid).Collection("reports")
	out := []Report{}
	it := col.OrderBy("createdAt", firestore.Desc).Limit(40).Documents(r.Context())
	defer it.Stop()
	for {
		d, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Println("reports:", err)
			http.Error(w, "db error", 500)
			return
		}
		var rep Report
		if d.DataTo(&rep) == nil {
			rep.ID = d.Ref.ID
			out = append(out, rep)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *server) evaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var in evalIn
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	col := s.db.Collection("users").Doc(uid).Collection("reports")
	start := time.Now().UTC().Truncate(24 * time.Hour)
	docs, err := col.Where("createdAt", ">=", start).Limit(dailyEvals).Documents(r.Context()).GetAll()
	if err != nil {
		log.Println("count:", err)
		http.Error(w, "db error", 500)
		return
	}
	if len(docs) >= dailyEvals {
		http.Error(w, fmt.Sprintf("Daily limit reached (%d evaluations). Try again tomorrow.", dailyEvals), 429)
		return
	}
	aiRuns := 0
	for _, d := range docs {
		if b, _ := d.Data()["aiUsed"].(bool); b {
			aiRuns++
		}
	}
	var notes []string
	if in.UseAI && aiRuns >= dailyAI {
		in.UseAI = false
		notes = append(notes, "Daily AI limit reached, so this run used measurements only.")
	}

	ctx, cancel := context.WithTimeout(r.Context(), 85*time.Second)
	defer cancel()
	var rep *Report
	switch in.Kind {
	case "website":
		rep, err = s.evalWebsite(ctx, in)
	case "social":
		rep, err = s.evalSocial(ctx, in)
	case "app":
		rep, err = s.evalApp(ctx, in)
	case "youtube":
		rep, err = s.evalYouTube(ctx, in)
	case "google":
		rep, err = s.evalGoogle(ctx, uid, in)
	default:
		err = userErr("Unknown evaluation type")
	}
	if err != nil {
		var ue userErr
		if errors.As(err, &ue) {
			http.Error(w, ue.Error(), 400)
			return
		}
		log.Println("evaluate:", err)
		http.Error(w, "Evaluation failed. Try again.", 502)
		return
	}
	rep.Kind = in.Kind
	rep.Notes = append(rep.Notes, notes...)
	rep.AIUsed = in.UseAI
	rep.CreatedAt = time.Now()
	ref, _, err := col.Add(r.Context(), rep)
	if err != nil {
		log.Println("save:", err)
		http.Error(w, "db error", 500)
		return
	}
	rep.ID = ref.ID
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rep)
}
