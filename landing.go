package main

// Public landing page back end: the instant website check (no sign-in), demo requests (saved, sent to the CRM and e-mailed to you),
// and the small public config the page needs.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	pubMu   sync.Mutex
	pubHits = map[string][]time.Time{}
	gradeMu sync.Mutex
	gradeC  = map[string]gradeOut{}
	demoWS  struct {
		sync.Mutex
		wid string
		at  time.Time
	}
	reEmail = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]{2,}$`)
)

// pubLimited allows at most max calls per window for a key (an IP address plus a purpose).
func pubLimited(key string, max int, window time.Duration) bool {
	pubMu.Lock()
	defer pubMu.Unlock()
	now := time.Now()
	keep := pubHits[key][:0:0]
	for _, t := range pubHits[key] {
		if now.Sub(t) < window {
			keep = append(keep, t)
		}
	}
	if len(keep) >= max {
		pubHits[key] = keep
		return true
	}
	pubHits[key] = append(keep, now)
	if len(pubHits) > 8000 {
		for k, v := range pubHits {
			if len(v) == 0 || now.Sub(v[len(v)-1]) > window {
				delete(pubHits, k)
			}
		}
	}
	return false
}

type gradeIssue struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Status string `json:"status"`
	Impact int    `json:"impact"`
	Fix    string `json:"fix,omitempty"`
}

type gradeOut struct {
	Domain     string         `json:"domain"`
	Score      int            `json:"score"`
	Pass       int            `json:"pass"`
	Warn       int            `json:"warn"`
	Fail       int            `json:"fail"`
	Total      int            `json:"total"`
	Categories map[string]int `json:"categories"`
	Issues     []gradeIssue   `json:"issues"`
	Hidden     int            `json:"hidden"`
	at         time.Time
}

// POST /api/public/grade {url}: a fast, AI-free website check. Shows the score and the biggest problems; the full list of fixes needs an account.
func (s *server) publicGrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct{ URL string }
	if !readBody(w, r, &in, 2<<10) {
		return
	}
	target, domain, err := normalizeURL(strings.TrimSpace(in.URL))
	if err != nil {
		http.Error(w, "Enter a website address like example.com", 400)
		return
	}
	gradeMu.Lock()
	if c, ok := gradeC[domain]; ok && time.Since(c.at) < time.Hour {
		gradeMu.Unlock()
		jsonOut(w, c)
		return
	}
	gradeMu.Unlock()
	ip := clientIP(r)
	if pubLimited("grade:"+ip, 6, time.Hour) || pubLimited("grade:all", 240, time.Hour) {
		http.Error(w, "Lots of people are checking sites right now. Try again in a few minutes, or create a free account for unlimited checks.", 429)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	rep, err := s.evalWebsite(ctx, evalIn{Kind: "website", URL: target, UseAI: false})
	if err != nil || rep == nil {
		http.Error(w, "We could not reach that website. Check the address and try again.", 400)
		return
	}
	out := gradeOut{Domain: domain, Score: rep.Score, Categories: rep.Categories, Total: len(rep.Checks), at: time.Now()}
	var bad []Check
	for _, c := range rep.Checks {
		switch c.Status {
		case "pass":
			out.Pass++
		case "warn":
			out.Warn++
			bad = append(bad, c)
		default:
			out.Fail++
			bad = append(bad, c)
		}
	}
	sort.SliceStable(bad, func(i, j int) bool {
		if (bad[i].Status == "fail") != (bad[j].Status == "fail") {
			return bad[i].Status == "fail"
		}
		return bad[i].Impact > bad[j].Impact
	})
	for i, c := range bad {
		if i >= 3 {
			out.Hidden = len(bad) - 3
			break
		}
		gi := gradeIssue{Name: c.Name, Value: c.Value, Status: c.Status, Impact: c.Impact}
		if i == 0 {
			gi.Fix = c.Fix // one real fix as a taste; the rest are in the account
		}
		out.Issues = append(out.Issues, gi)
	}
	gradeMu.Lock()
	gradeC[domain] = out
	if len(gradeC) > 500 {
		for k, v := range gradeC {
			if time.Since(v.at) > time.Hour {
				delete(gradeC, k)
			}
		}
	}
	gradeMu.Unlock()
	sum := sha256.Sum256([]byte(ip))
	go s.db.Collection("public_grades").NewDoc().Set(context.Background(), map[string]any{"domain": domain, "score": out.Score, "ip": hex.EncodeToString(sum[:6]), "at": time.Now()})
	jsonOut(w, out)
}

// demoWorkspace finds the CRM workspace of the first admin, so demo requests land there as leads. Cached for ten minutes.
func (s *server) demoWorkspace(ctx context.Context) string {
	demoWS.Lock()
	defer demoWS.Unlock()
	if demoWS.wid != "" && time.Since(demoWS.at) < 10*time.Minute {
		return demoWS.wid
	}
	if v := strings.TrimSpace(env("DEMO_WORKSPACE")); v != "" && reHexID.MatchString(v) {
		demoWS.wid, demoWS.at = v, time.Now()
		return v
	}
	for _, e := range strings.Split(env("ADMIN_EMAILS"), ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		u, err := s.auth.GetUserByEmail(ctx, e)
		if err != nil {
			continue
		}
		if d, err := s.db.Collection("users").Doc(u.UID).Collection("private").Doc("crm").Get(ctx); err == nil {
			if wid := sv(d.Data(), "wid"); wid != "" {
				demoWS.wid, demoWS.at = wid, time.Now()
				return wid
			}
		}
	}
	return ""
}

var demoFocus = map[string]string{"leads": "Get more leads", "chats": "Answer chats faster", "post": "Post and advertise", "deals": "Track and close deals", "all": "Everything in one place"}

// POST /api/public/demo: someone asks for a walkthrough.
func (s *server) publicDemo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var in struct {
		Name, Email, Company, Website, Phone, Focus, Hp string
		Score                                           int
	}
	if !readBody(w, r, &in, 8<<10) {
		return
	}
	ip := clientIP(r)
	if in.Hp != "" { // honeypot: pretend it worked
		jsonOut(w, map[string]any{"ok": true})
		return
	}
	in.Name, in.Email, in.Company = clip(strings.TrimSpace(in.Name), 100), strings.ToLower(clip(strings.TrimSpace(in.Email), 160)), clip(strings.TrimSpace(in.Company), 120)
	in.Website, in.Phone = clip(strings.TrimSpace(in.Website), 160), clip(strings.TrimSpace(in.Phone), 40)
	if len([]rune(in.Name)) < 2 || !reEmail.MatchString(in.Email) {
		http.Error(w, "Please enter your name and a valid work email.", 400)
		return
	}
	if pubLimited("demo:"+ip, 4, time.Hour) {
		http.Error(w, "Too many requests from this connection. Please try again later.", 429)
		return
	}
	focus := demoFocus[in.Focus]
	if focus == "" {
		focus = "Not specified"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	now := time.Now()
	doc := map[string]any{"name": in.Name, "email": in.Email, "company": in.Company, "website": in.Website, "phone": in.Phone, "focus": focus, "gradeScore": in.Score, "createdAt": now, "status": "new"}
	s.db.Collection("demo_requests").NewDoc().Set(ctx, doc)
	if wid := s.demoWorkspace(ctx); wid != "" {
		note := "Demo request. Wants to: " + focus
		if in.Website != "" {
			note += ". Website: " + in.Website
		}
		if in.Score > 0 {
			note += " (free check score " + itoa(in.Score) + "/100)"
		}
		f := map[string]any{"name": in.Name, "email": in.Email, "phone": in.Phone, "company": in.Company, "source": "demo_request", "sourceRef": "demo:" + in.Email}
		s.crmInboundID(ctx, wid, f, note)
	}
	booking := strings.TrimSpace(env("DEMO_BOOKING_URL"))
	wa := reDigits.ReplaceAllString(env("DEMO_WHATSAPP"), "")
	go s.demoMails(in.Name, in.Email, in.Company, in.Website, in.Phone, focus, in.Score, booking)
	jsonOut(w, map[string]any{"ok": true, "booking": booking, "whatsapp": wa})
}

func (s *server) demoMails(name, email, company, website, phone, focus string, score int, booking string) {
	if !mailConfigured() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	to := strings.TrimSpace(env("DEMO_NOTIFY_EMAIL"))
	if to == "" {
		to = strings.TrimSpace(strings.Split(env("ADMIN_EMAILS"), ",")[0])
	}
	e := html.EscapeString
	if to != "" {
		body := "<p><b>New demo request</b></p><p>" + e(name) + (func() string {
			if company != "" {
				return " (" + e(company) + ")"
			}
			return ""
		})() + "<br>Email: " + e(email) + "<br>Phone / WhatsApp: " + e(phone) + "<br>Website: " + e(website) + "<br>Wants to: " + e(focus) + "</p><p>Reply within the hour if you can: a fast reply is the biggest factor in whether they book.</p>"
		sendMailH(ctx, to, "Demo request: "+name, body, "Demo request from "+name+" <"+email+">. Phone: "+phone+". Website: "+website+". Wants to: "+focus, map[string]string{"Reply-To": email}, false)
	}
	first := strings.Fields(name)[0]
	next := "We will message you shortly to agree a time for a 20-minute walkthrough on your own business."
	nextH := e(next)
	if booking != "" {
		next += " Or pick a time right now: " + booking
		nextH += ` Or <a href="` + e(booking) + `">pick a time right now</a>.`
	}
	sendMail(ctx, email, "We got your request, "+first, "<p>Hi "+e(first)+",</p><p>Thanks for asking about ficx. "+nextH+"</p><p>In the walkthrough we will look at what you want to fix first (<b>"+e(focus)+"</b>), show you how ficx does it, and answer your questions. No card, no pressure.</p><p>ficx</p>", "Hi "+first+", thanks for asking about ficx. "+next)
}

// GET /api/public/config: what the landing page needs to show.
func (s *server) publicConfig(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, map[string]any{"booking": strings.TrimSpace(env("DEMO_BOOKING_URL")), "whatsapp": reDigits.ReplaceAllString(env("DEMO_WHATSAPP"), ""), "starter": starterOn()})
}
