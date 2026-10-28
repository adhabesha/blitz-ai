package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
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

const dailyLimit = 5 // analyses per user per day, protects the free Gemini quota

var (
	errRate = errors.New("rate limited")
	domRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,98}\.[a-z]{2,}$`)
)

type server struct {
	auth       *auth.Client
	db         *firestore.Client
	tpl        *template.Template
	cfg        map[string]string
	gkey, model string
}

type project struct {
	ID        string    `json:"id" firestore:"-"`
	Domain    string    `json:"domain" firestore:"domain"`
	Score     int       `json:"score" firestore:"score"`
	Summary   string    `json:"summary" firestore:"summary"`
	Tips      []string  `json:"tips" firestore:"tips"`
	CreatedAt time.Time `json:"createdAt" firestore:"createdAt"`
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
	mux.HandleFunc("/api/projects", s.projects)

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

func cleanDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	d = strings.TrimPrefix(d, "www.")
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	return d
}

// analyze asks Gemini (free tier) for an AI-visibility estimate. One call per analysis.
func (s *server) analyze(ctx context.Context, domain string) (int, string, []string, error) {
	prompt := fmt.Sprintf(`You are an AI-search visibility (GEO) analyst. For the website "%s", estimate how likely AI assistants (ChatGPT, Gemini, Perplexity) are to mention or cite it for its category, using only what you know. If you don't recognise it, give a low score and say so. Reply ONLY with JSON: {"score": integer 0-100, "summary": "two sentences", "tips": ["three short actionable tips"]}`, domain)
	body, _ := json.Marshal(map[string]any{
		"contents":         []any{map[string]any{"parts": []any{map[string]string{"text": prompt}}}},
		"generationConfig": map[string]any{"responseMimeType": "application/json"},
	})
	url := "https://generativelanguage.googleapis.com/v1beta/models/" + s.model + ":generateContent"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", s.gkey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return 0, "", nil, errRate
	}
	if resp.StatusCode != http.StatusOK {
		return 0, "", nil, fmt.Errorf("gemini status %d", resp.StatusCode)
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct{ Text string } `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return 0, "", nil, errors.New("empty gemini reply")
	}
	var r struct {
		Score   int      `json:"score"`
		Summary string   `json:"summary"`
		Tips    []string `json:"tips"`
	}
	if err := json.Unmarshal([]byte(out.Candidates[0].Content.Parts[0].Text), &r); err != nil {
		return 0, "", nil, err
	}
	if r.Score < 0 {
		r.Score = 0
	}
	if r.Score > 100 {
		r.Score = 100
	}
	return r.Score, r.Summary, r.Tips, nil
}

func (s *server) projects(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	col := s.db.Collection("users").Doc(uid).Collection("projects")

	switch r.Method {
	case http.MethodGet:
		out := []project{}
		it := col.OrderBy("createdAt", firestore.Desc).Limit(50).Documents(r.Context())
		defer it.Stop()
		for {
			d, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				http.Error(w, "db error", 500)
				return
			}
			var p project
			d.DataTo(&p)
			p.ID = d.Ref.ID
			out = append(out, p)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)

	case http.MethodPost:
		var in struct{ Domain string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		d := cleanDomain(in.Domain)
		if !domRe.MatchString(d) {
			http.Error(w, "Enter a valid domain like example.com", 400)
			return
		}
		start := time.Now().UTC().Truncate(24 * time.Hour)
		docs, err := col.Where("createdAt", ">=", start).Limit(dailyLimit).Documents(r.Context()).GetAll()
		if err != nil {
			http.Error(w, "db error", 500)
			return
		}
		if len(docs) >= dailyLimit {
			http.Error(w, fmt.Sprintf("Daily limit reached (%d analyses). Try again tomorrow.", dailyLimit), 429)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
		defer cancel()
		score, summary, tips, err := s.analyze(ctx, d)
		if errors.Is(err, errRate) {
			http.Error(w, "The free AI quota is busy right now. Try again later.", 429)
			return
		}
		if err != nil {
			log.Println("analyze:", err)
			http.Error(w, "AI check failed. Try again.", 502)
			return
		}
		p := project{Domain: d, Score: score, Summary: summary, Tips: tips, CreatedAt: time.Now()}
		ref, _, err := col.Add(r.Context(), p)
		if err != nil {
			http.Error(w, "db error", 500)
			return
		}
		p.ID = ref.ID
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)

	default:
		http.Error(w, "method not allowed", 405)
	}
}
