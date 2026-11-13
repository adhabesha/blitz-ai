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
	opt := option.WithCredentialsJSON([]byte(env("FIREBASE_CREDENTIALS")))
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: env("FIREBASE_PROJECT_ID")}, opt)
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
	model := env("GEMINI_MODEL")
	if model == "" {
		model = "gemini-3.5-flash-lite"
	}
	s := &server{
		auth: a, db: db, gkey: env("GEMINI_API_KEY"), model: model,
		tpl: template.Must(template.ParseFS(assets, "templates/*.html")),
		cfg: map[string]string{
			"APIKey":     env("FIREBASE_WEB_API_KEY"),
			"AuthDomain": env("FIREBASE_AUTH_DOMAIN"),
			"ProjectID":  env("FIREBASE_PROJECT_ID"),
		},
	}

	s.loadConfig(ctx)
	go func() {
		for range time.Tick(2 * time.Minute) {
			c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			s.loadConfig(c)
			cancel()
		}
	}()

	go s.scheduleLoop()
	go s.outboundLoop()

	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.FS(assets)))
	mux.HandleFunc("/", s.page("landing.html"))
	mux.HandleFunc("/dashboard", s.page("dashboard.html"))
	mux.HandleFunc("/app", s.page("auth.html"))
	mux.HandleFunc("/admin", s.page("admin.html"))
	mux.HandleFunc("/pending", s.page("pending.html"))
	mux.HandleFunc("/api/approval", s.approvalState)
	mux.HandleFunc("/api/admin/users", s.adminUsers)
	mux.HandleFunc("/api/admin/users/set", s.adminUserSet)
	mux.HandleFunc("/api/me", s.me)
	mux.HandleFunc("/api/profile", s.saveProfile)
	mux.HandleFunc("/api/admin/config", s.adminConfig)
	mux.HandleFunc("/api/admin/test", s.adminTest)
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
	mux.HandleFunc("/api/viral/research", s.viralResearch)
	mux.HandleFunc("/api/ads/accounts", s.adsAccountsHandler)
	mux.HandleFunc("/api/ads/launch", s.adsLaunch)
	mux.HandleFunc("/api/ads/action", s.adsAction)
	mux.HandleFunc("/api/adcreative", s.adCreative)
	mux.HandleFunc("/crm", s.page("crm.html"))
	mux.HandleFunc("/crm/form/", s.crmPublicForm)
	mux.HandleFunc("/api/crm/me", s.crmMe)
	mux.HandleFunc("/api/crm/workspace", s.crmCreateWorkspace)
	mux.HandleFunc("/api/crm/list", s.crmList)
	mux.HandleFunc("/api/crm/save", s.crmSave)
	mux.HandleFunc("/api/crm/delete", s.crmDelete)
	mux.HandleFunc("/api/crm/activities", s.crmActivities)
	mux.HandleFunc("/api/crm/activity", s.crmAddActivity)
	mux.HandleFunc("/api/crm/lead/convert", s.crmConvert)
	mux.HandleFunc("/api/crm/leads/import", s.crmImport)
	mux.HandleFunc("/api/crm/team", s.crmTeam)
	mux.HandleFunc("/api/crm/member/save", s.crmMemberSave)
	mux.HandleFunc("/api/crm/member/remove", s.crmMemberRemove)
	mux.HandleFunc("/api/crm/invite", s.crmInvite)
	mux.HandleFunc("/api/crm/invite/cancel", s.crmInviteCancel)
	mux.HandleFunc("/api/crm/invite/resend", s.crmInviteResend)
	mux.HandleFunc("/privacy", s.privacyPage)
	mux.HandleFunc("/terms", s.termsPage)
	mux.HandleFunc("/data-deletion", s.deletionPage)
	mux.HandleFunc("/api/crm/outbound/settings", s.crmObSettings)
	mux.HandleFunc("/api/crm/prospects", s.crmProspects)
	mux.HandleFunc("/api/crm/prospect/search", s.crmProspectSearch)
	mux.HandleFunc("/api/crm/prospect/lookup", s.crmProspectLookup)
	mux.HandleFunc("/api/crm/prospect/promote", s.crmProspectPromote)
	mux.HandleFunc("/api/crm/prospect/delete", s.crmProspectDelete)
	mux.HandleFunc("/api/crm/prospect/edit", s.crmProspectEdit)
	mux.HandleFunc("/api/crm/prospect/find", s.crmProspectFind)
	mux.HandleFunc("/api/crm/outreach/draft", s.crmOutreachDraft)
	mux.HandleFunc("/api/crm/sequences", s.crmSequences)
	mux.HandleFunc("/api/crm/sequence/save", s.crmSequenceSave)
	mux.HandleFunc("/api/crm/sequence/delete", s.crmSequenceDelete)
	mux.HandleFunc("/api/crm/sequence/enroll", s.crmSequenceEnroll)
	mux.HandleFunc("/api/crm/sequence/enrollments", s.crmEnrollments)
	mux.HandleFunc("/api/crm/sequence/stop", s.crmEnrollStop)
	mux.HandleFunc("/api/crm/inbox", s.crmInbox)
	mux.HandleFunc("/api/crm/inbox/add", s.crmInboxAdd)
	mux.HandleFunc("/api/crm/inbox/handle", s.crmInboxHandle)
	mux.HandleFunc("/api/crm/inbox/reply", s.crmInboxReply)
	mux.HandleFunc("/api/crm/inbound/", s.crmMailInbound)
	mux.HandleFunc("/t/o/", s.trackOpen)
	mux.HandleFunc("/u/", s.unsubscribe)
	mux.HandleFunc("/api/crm/flows", s.crmFlows)
	mux.HandleFunc("/api/crm/flow/save", s.crmFlowSave)
	mux.HandleFunc("/api/crm/flow/delete", s.crmFlowDelete)
	mux.HandleFunc("/api/crm/chats", s.crmChats)
	mux.HandleFunc("/api/crm/chat/send", s.crmChatSend)
	mux.HandleFunc("/api/crm/chat/resume", s.crmChatResume)
	mux.HandleFunc("/api/crm/dept/save", s.crmDeptSave)
	mux.HandleFunc("/api/crm/dept/delete", s.crmDeptDelete)
	mux.HandleFunc("/api/crm/settings", s.crmSettings)
	mux.HandleFunc("/api/crm/dashboard", s.crmDashboard)
	mux.HandleFunc("/api/crm/ads", s.crmAds)
	mux.HandleFunc("/api/crm/integrations", s.crmIntegrations)
	mux.HandleFunc("/api/crm/integrations/rotate", s.crmRotate)
	mux.HandleFunc("/api/crm/meta/connect", s.crmMetaConnect)
	mux.HandleFunc("/api/crm/meta/sync", s.crmMetaSync)
	mux.HandleFunc("/api/crm/meta/webhook", s.crmMetaWebhook)
	mux.HandleFunc("/api/crm/hook/", s.crmHook)
	mux.HandleFunc("/api/publish", s.publish)
	mux.HandleFunc("/api/publish/status", s.publishStatus)
	mux.HandleFunc("/api/tiktok/creator", s.tiktokCreator)
	mux.HandleFunc("/api/publish/scheduled", s.scheduledList)
	mux.HandleFunc("/api/publish/scheduled/cancel", s.scheduledCancel)
	mux.HandleFunc("/media/", s.serveMedia)
	mux.HandleFunc("/api/social/status", s.socialStatus)
	mux.HandleFunc("/api/social/start", s.socialStart)
	mux.HandleFunc("/api/social/disconnect", s.socialDisconnect)
	mux.HandleFunc("/api/social/post/apply", s.applyPost)
	for _, p := range providers {
		mux.HandleFunc("/auth/"+p.ID+"/callback", s.socialCallback(p.ID))
	}
	mux.HandleFunc("/api/youtube/apply", s.applyYouTube)
	mux.HandleFunc("/api/youtube/channel/apply", s.applyYouTubeChannel)
	mux.HandleFunc("/api/site/status", s.siteStatus)
	mux.HandleFunc("/api/site/connect", s.siteConnect)
	mux.HandleFunc("/api/site/disconnect", s.siteDisconnect)
	mux.HandleFunc("/api/site/apply", s.siteApply)

	port := env("PORT")
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
	if !s.approved(r.Context(), t) {
		return "", errors.New("awaiting approval")
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
	case "adsperf":
		rep, err = s.evalAdsPerf(ctx, uid, in)
	case "keywords":
		rep, err = s.evalKeywords(ctx, in)
	case "competitors":
		rep, err = s.evalCompetitors(ctx, in)
	case "ads":
		rep, err = s.evalAds(ctx, in)
	case "campaigns":
		rep, err = s.evalCampaigns(ctx, uid, in)
	case "social_connected":
		rep, err = s.evalConnectedSocial(ctx, uid, in)
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
	layoutReport(rep)
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
