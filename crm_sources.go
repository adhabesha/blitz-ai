package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Lead sources: website form + webhook, Meta Lead Ads, Facebook/Instagram messages and comments.

var reHexID = regexp.MustCompile(`^[0-9a-f]{8,40}$`)

var (
	hookMu   sync.Mutex
	hookHits = map[string][]time.Time{}
)

func hookLimited(ip string, max int) bool {
	hookMu.Lock()
	defer hookMu.Unlock()
	now := time.Now()
	var keep []time.Time
	for _, t := range hookHits[ip] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= max {
		hookHits[ip] = keep
		return true
	}
	hookHits[ip] = append(keep, now)
	if len(hookHits) > 5000 {
		hookHits = map[string][]time.Time{ip: hookHits[ip]}
	}
	return false
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	return r.RemoteAddr
}

// pickOwner assigns new inbound leads round-robin to active sales people and managers when auto-assign is on.
func (s *server) crmPickOwner(ctx context.Context, wid string) string {
	wd, err := s.crmWS(wid).Get(ctx)
	if err != nil || wd.Data()["autoAssign"] != true {
		return ""
	}
	c := &crmReq{s: s, ctx: ctx, wid: wid}
	var el []string
	for _, m := range c.members() {
		if sv(m, "status") != "disabled" && (sv(m, "role") == "sales" || sv(m, "role") == "manager") {
			el = append(el, sv(m, "uid"))
		}
	}
	if len(el) == 0 {
		return ""
	}
	sort.Strings(el)
	rr := int(nv(wd.Data(), "rr"))
	s.crmWS(wid).Set(ctx, map[string]any{"rr": rr + 1}, firestoreMerge())
	return el[rr%len(el)]
}

func (s *server) crmInbound(ctx context.Context, wid string, f map[string]any, note string) {
	if sv(f, "ownerUid") == "" {
		f["ownerUid"] = s.crmPickOwner(ctx, wid)
	}
	s.crmAddLead(ctx, wid, f, "", note)
}

// hookLog keeps the last 30 things Meta sent us per workspace (or "_unmatched") so problems are visible.
func (s *server) hookLog(ctx context.Context, wid, kind, detail string) {
	if wid == "" {
		wid = "_unmatched"
	}
	ref := s.db.Collection("crm_hooklog").Doc(wid)
	var items []any
	if d, err := ref.Get(ctx); err == nil {
		items, _ = d.Data()["items"].([]any)
	}
	items = append(items, map[string]any{"at": time.Now(), "kind": clip(kind, 60), "detail": clip(detail, 240)})
	if len(items) > 30 {
		items = items[len(items)-30:]
	}
	ref.Set(ctx, map[string]any{"items": items})
}

func (s *server) hookLogOf(ctx context.Context, wid string) []map[string]any {
	var out []map[string]any
	for _, id := range []string{wid, "_unmatched"} {
		d, err := s.db.Collection("crm_hooklog").Doc(id).Get(ctx)
		if err != nil {
			continue
		}
		its, _ := d.Data()["items"].([]any)
		for _, x := range its {
			if m, ok := x.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return timeOf(out[i]["at"]).After(timeOf(out[j]["at"])) })
	if len(out) > 25 {
		out = out[:25]
	}
	return out
}

func (s *server) crmInboundID(ctx context.Context, wid string, f map[string]any, note string) string {
	if sv(f, "ownerUid") == "" {
		f["ownerUid"] = s.crmPickOwner(ctx, wid)
	}
	id, _, _ := s.crmAddLead(ctx, wid, f, "", note)
	return id
}

// ---------------------------------------------------------------- website form + webhook

func (s *server) crmHook(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/crm/hook/"), "/"), "/")
	if len(parts) != 2 || !reHexID.MatchString(parts[0]) || !reHexID.MatchString(parts[1]) {
		http.Error(w, "not found", 404)
		return
	}
	if hookLimited(clientIP(r), 20) {
		http.Error(w, "Too many requests. Try again in a minute.", 429)
		return
	}
	wd, err := s.crmWS(parts[0]).Get(r.Context())
	if err != nil || subtle.ConstantTimeCompare([]byte(sv(wd.Data(), "hookSecret")), []byte(parts[1])) != 1 {
		http.Error(w, "not found", 404)
		return
	}
	in := map[string]any{}
	if strings.Contains(r.Header.Get("Content-Type"), "json") {
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in) != nil {
			http.Error(w, "bad request", 400)
			return
		}
	} else {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		if r.ParseForm() != nil {
			http.Error(w, "bad request", 400)
			return
		}
		for k, v := range r.Form {
			if len(v) > 0 {
				in[k] = v[0]
			}
		}
	}
	str := func(k ...string) string {
		for _, x := range k {
			if v, ok := in[x].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	if str("hp", "_gotcha") != "" { // honeypot field filled by bots
		jsonOut(w, map[string]bool{"ok": true})
		return
	}
	f := map[string]any{
		"name": clip(str("name", "full_name", "fullName"), 120), "email": clip(str("email"), 160), "phone": clip(str("phone", "phone_number", "tel"), 40),
		"company": clip(str("company", "company_name"), 120), "notes": ctext(str("message", "notes", "comment"), 3000), "source": "web_form",
		"campaign": clip(str("utm_campaign", "campaign"), 160), "formName": clip(firstNonEmptyStr(str("form", "page"), r.Header.Get("Referer")), 160),
	}
	if s := clip(str("source"), 40); s != "" {
		f["source"] = strings.ToLower(strings.ReplaceAll(s, " ", "_"))
	}
	if sv(f, "email") == "" && sv(f, "phone") == "" {
		http.Error(w, "Enter an email address or a phone number.", 400)
		return
	}
	if sv(f, "email") != "" && !reEmailOK.MatchString(sv(f, "email")) {
		http.Error(w, "Enter a valid email address.", 400)
		return
	}
	if sv(f, "name") == "" {
		f["name"] = firstNonEmptyStr(sv(f, "email"), sv(f, "phone"))
	}
	s.crmInbound(r.Context(), parts[0], f, sv(f, "notes"))
	jsonOut(w, map[string]bool{"ok": true})
}

func (s *server) crmPublicForm(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/crm/form/"), "/"), "/")
	if len(parts) != 2 || !reHexID.MatchString(parts[0]) || !reHexID.MatchString(parts[1]) {
		http.NotFound(w, r)
		return
	}
	wd, err := s.crmWS(parts[0]).Get(r.Context())
	if err != nil || subtle.ConstantTimeCompare([]byte(sv(wd.Data(), "hookSecret")), []byte(parts[1])) != 1 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Contact %[1]s</title>
<style>body{font-family:Inter,system-ui,sans-serif;background:#f2f8f8;margin:0;padding:24px;color:#0e1834}form{max-width:460px;margin:0 auto;background:#fff;border-radius:18px;padding:26px;box-shadow:0 12px 34px rgba(14,24,52,.1);display:grid;gap:12px}h1{margin:0 0 4px;font-size:22px}label{font-size:13px;color:#4f5f80;display:grid;gap:5px}input,textarea{font:inherit;padding:10px 12px;border:1px solid #d6deef;border-radius:10px}button{background:#da602c;color:#fff;border:0;border-radius:12px;padding:12px;font:inherit;font-weight:600;cursor:pointer}.m{font-size:14px}.hp{position:absolute;left:-9999px}</style></head><body>
<form id="f"><h1>Contact %[1]s</h1><label>Your name<input name="name" required maxlength="120"></label><label>Email<input name="email" type="email" maxlength="160"></label><label>Phone<input name="phone" maxlength="40"></label><label>Company (optional)<input name="company" maxlength="120"></label><label>How can we help?<textarea name="message" rows="4" maxlength="2000"></textarea></label>
<input class="hp" name="hp" tabindex="-1" autocomplete="off"><button>Send</button><div class="m" id="m"></div></form>
<script>document.getElementById("f").onsubmit=async e=>{e.preventDefault();const m=document.getElementById("m"),d=Object.fromEntries(new FormData(e.target));d.source="web_form";d.form=location.href;
if(!d.email&&!d.phone){m.textContent="Please enter an email or a phone number.";return}
try{const r=await fetch("/api/crm/hook/%[2]s/%[3]s",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(d)});if(!r.ok)throw new Error(await r.text());e.target.innerHTML="<h1>Thank you!</h1><p>We received your message and will contact you soon.</p>"}catch(x){m.textContent=x.message||"Something went wrong."}}</script></body></html>`,
		html.EscapeString(sv(wd.Data(), "name")), parts[0], parts[1])
}

// ---------------------------------------------------------------- integrations screen

func metaVerifyToken() string {
	if v := env("META_WEBHOOK_VERIFY_TOKEN"); v != "" {
		return v
	}
	h := sha256.Sum256([]byte("blitz-crm|" + env("FACEBOOK_APP_SECRET")))
	return hex.EncodeToString(h[:12])
}

func (s *server) crmIntegrations(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("integrations", 'v') {
		c.deny(w, "view lead sources")
		return
	}
	base := strings.TrimRight(env("BASE_URL"), "/")
	if base == "" {
		base = "https://" + r.Host
	}
	secret := sv(c.ws, "hookSecret")
	hook := base + "/api/crm/hook/" + c.wid + "/" + secret
	form := base + "/crm/form/" + c.wid + "/" + secret
	pages := []map[string]any{}
	it := s.db.Collection("crm_pages").Where("wid", "==", c.wid).Documents(c.ctx)
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		x := d.Data()
		pages = append(pages, map[string]any{"id": d.Ref.ID, "name": x["name"], "kind": x["kind"], "lastSync": x["lastSync"]})
	}
	it.Stop()
	jsonOut(w, map[string]any{
		"webhookUrl": hook, "formUrl": form,
		"embed":    `<iframe src="` + form + `" style="width:100%;max-width:520px;height:560px;border:0" title="Contact form"></iframe>`,
		"curl":     `curl -X POST "` + hook + `" -H "Content-Type: application/json" -d '{"name":"Jane Doe","email":"jane@example.com","phone":"+251911000000","message":"I want a quote"}'`,
		"hookLog": s.hookLogOf(c.ctx, c.wid), "metaPages": pages, "metaOwner": sv(c.ws, "metaOwner"),
		"metaWebhook": map[string]any{"url": base + "/api/crm/meta/webhook", "verifyToken": metaVerifyToken(), "appSecretSet": env("FACEBOOK_APP_SECRET") != ""},
	})
}

func (s *server) crmRotate(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("integrations", 'e') {
		c.deny(w, "change lead sources")
		return
	}
	s.crmWS(c.wid).Set(c.ctx, map[string]any{"hookSecret": randHex(12)}, firestoreMerge())
	jsonOut(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------- Meta

type crmFBPage struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Token string `json:"access_token"`
	IG    *struct {
		ID string `json:"id"`
	} `json:"instagram_business_account"`
}

func (s *server) fbUserPages(ctx context.Context, uid string) ([]crmFBPage, error) {
	t, err := s.validToken(ctx, uid, provOf("facebook"))
	if err != nil {
		return nil, err
	}
	var d struct {
		Data []crmFBPage `json:"data"`
	}
	if err := getJSON(ctx, "https://graph.facebook.com/v21.0/me/accounts?"+url.Values{"fields": {"id,name,access_token,instagram_business_account"}, "limit": {"50"}, "access_token": {t.Access}}.Encode(), &d); err != nil {
		return nil, err
	}
	return d.Data, nil
}

// connect links the Pages the signed-in person connected to ficx (Connected social > Facebook) to this workspace.
func (s *server) crmMetaConnect(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("integrations", 'e') {
		c.deny(w, "connect lead sources")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	pages, err := s.fbUserPages(ctx, c.uid)
	if err != nil {
		http.Error(w, "Could not read your Facebook Pages: "+trunc(err.Error(), 160)+" Connect Facebook first under Connected social.", 400)
		return
	}
	if len(pages) == 0 {
		http.Error(w, "No Facebook Pages were shared with ficx. Reconnect Facebook and tick your Page.", 400)
		return
	}
	var warn []string
	for _, p := range pages {
		s.db.Collection("crm_pages").Doc(p.ID).Set(ctx, map[string]any{"wid": c.wid, "ownerUid": c.uid, "name": p.Name, "kind": "facebook"})
		if p.IG != nil && p.IG.ID != "" {
			s.db.Collection("crm_pages").Doc(p.IG.ID).Set(ctx, map[string]any{"wid": c.wid, "ownerUid": c.uid, "name": p.Name + " (Instagram)", "kind": "instagram", "pageId": p.ID})
		}
		var out struct {
			Success bool `json:"success"`
		}
		if err := postForm(ctx, "https://graph.facebook.com/v21.0/"+p.ID+"/subscribed_apps", url.Values{"subscribed_fields": {"leadgen,messages,feed"}, "access_token": {p.Token}}, &out); err != nil {
			warn = append(warn, p.Name+": "+trunc(err.Error(), 140))
		}
		if p.IG != nil && p.IG.ID != "" {
			var o2 struct {
				Success bool `json:"success"`
			}
			if err := postForm(ctx, "https://graph.facebook.com/v21.0/"+p.IG.ID+"/subscribed_apps", url.Values{"subscribed_fields": {"comments,messages"}, "access_token": {p.Token}}, &o2); err != nil {
				warn = append(warn, "Instagram "+p.Name+": "+trunc(err.Error(), 140))
			}
		}
	}
	s.crmWS(c.wid).Set(ctx, map[string]any{"metaOwner": c.uid}, firestoreMerge())
	jsonOut(w, map[string]any{"pages": len(pages), "warnings": warn})
}

func fieldVal(fd []struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}, names ...string) string {
	for _, n := range names {
		for _, f := range fd {
			if strings.EqualFold(f.Name, n) && len(f.Values) > 0 {
				return f.Values[0]
			}
		}
	}
	return ""
}

type metaLead struct {
	ID           string `json:"id"`
	CreatedTime  string `json:"created_time"`
	AdName       string `json:"ad_name"`
	CampaignName string `json:"campaign_name"`
	FormID       string `json:"form_id"`
	FieldData    []struct {
		Name   string   `json:"name"`
		Values []string `json:"values"`
	} `json:"field_data"`
}

func (s *server) crmPageToken(ctx context.Context, pageID string) (wid, tok string, err error) {
	d, e := s.db.Collection("crm_pages").Doc(pageID).Get(ctx)
	if e != nil {
		return "", "", e
	}
	x := d.Data()
	wid = sv(x, "wid")
	if sv(x, "kind") == "whatsapp" {
		return wid, "wa:" + sv(x, "token"), nil
	}
	if sv(x, "kind") == "instagram" {
		pageID = sv(x, "pageId")
	}
	t, e := s.validToken(ctx, sv(x, "ownerUid"), provOf("facebook"))
	if e != nil {
		return wid, "", e
	}
	var p struct {
		Token string `json:"access_token"`
	}
	if e := getJSON(ctx, "https://graph.facebook.com/v21.0/"+pageID+"?"+url.Values{"fields": {"access_token"}, "access_token": {t.Access}}.Encode(), &p); e != nil {
		return wid, "", e
	}
	return wid, p.Token, nil
}

// sync pulls existing lead-form submissions from the connected Pages.
func (s *server) crmMetaSync(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("integrations", 'e') {
		c.deny(w, "sync lead sources")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 70*time.Second)
	defer cancel()
	it := s.db.Collection("crm_pages").Where("wid", "==", c.wid).Documents(ctx)
	type pg struct{ id, name string }
	var pages []pg
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		if sv(d.Data(), "kind") == "facebook" {
			pages = append(pages, pg{d.Ref.ID, sv(d.Data(), "name")})
		}
	}
	it.Stop()
	if len(pages) == 0 {
		http.Error(w, "Connect your Facebook Pages first.", 400)
		return
	}
	added, seen := 0, 0
	var warn []string
	for _, p := range pages {
		_, tok, err := s.crmPageToken(ctx, p.id)
		if err != nil || tok == "" {
			warn = append(warn, p.name+": could not get access to the Page")
			continue
		}
		var forms struct {
			Data []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"data"`
		}
		if err := getJSON(ctx, "https://graph.facebook.com/v21.0/"+p.id+"/leadgen_forms?"+url.Values{"fields": {"id,name"}, "limit": {"25"}, "access_token": {tok}}.Encode(), &forms); err != nil {
			warn = append(warn, p.name+": "+trunc(err.Error(), 140)+" (the leads_retrieval permission and Page admin rights are required)")
			continue
		}
		for _, fm := range forms.Data {
			var ls struct {
				Data []metaLead `json:"data"`
			}
			if err := getJSON(ctx, "https://graph.facebook.com/v21.0/"+fm.ID+"/leads?"+url.Values{"fields": {"id,created_time,field_data,ad_name,campaign_name,form_id"}, "limit": {"100"}, "access_token": {tok}}.Encode(), &ls); err != nil {
				warn = append(warn, fm.Name+": "+trunc(err.Error(), 120))
				continue
			}
			for _, l := range ls.Data {
				seen++
				_, dup, err := s.crmAddLeadMeta(ctx, c.wid, l, fm.Name)
				if err == nil && !dup {
					added++
				}
			}
		}
		s.db.Collection("crm_pages").Doc(p.id).Set(ctx, map[string]any{"lastSync": time.Now()}, firestoreMerge())
	}
	jsonOut(w, map[string]any{"added": added, "checked": seen, "warnings": warn})
}

func (s *server) crmAddLeadMeta(ctx context.Context, wid string, l metaLead, form string) (string, bool, error) {
	fd := l.FieldData
	name := firstNonEmptyStr(fieldVal(fd, "full_name", "name"), strings.TrimSpace(fieldVal(fd, "first_name")+" "+fieldVal(fd, "last_name")))
	f := map[string]any{
		"name": clip(name, 120), "email": clip(fieldVal(fd, "email"), 160), "phone": clip(fieldVal(fd, "phone_number", "phone"), 40), "company": clip(fieldVal(fd, "company_name", "company"), 120),
		"city": clip(fieldVal(fd, "city"), 80), "source": "meta_lead_ad", "sourceRef": "meta:" + l.ID, "campaign": clip(l.CampaignName, 160), "adName": clip(l.AdName, 160), "formName": clip(form, 160),
	}
	if sv(f, "name") == "" {
		f["name"] = firstNonEmptyStr(sv(f, "email"), sv(f, "phone"), "Meta lead")
	}
	f["ownerUid"] = s.crmPickOwner(ctx, wid)
	return s.crmAddLead(ctx, wid, f, "", "Lead form submitted"+ifs(l.CampaignName != "", " (campaign "+l.CampaignName+")", ""))
}

// Meta webhook: lead forms, Messenger/Instagram messages and comments become leads.
func (s *server) crmMetaWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		if q.Get("hub.mode") == "subscribe" && subtle.ConstantTimeCompare([]byte(q.Get("hub.verify_token")), []byte(metaVerifyToken())) == 1 {
			s.hookLog(context.Background(), "", "Webhook verified", "Meta confirmed the callback URL and verify token.")
			w.Write([]byte(q.Get("hub.challenge")))
			return
		}
		http.Error(w, "forbidden", 403)
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	secret := env("FACEBOOK_APP_SECRET")
	if secret == "" {
		s.hookLog(context.Background(), "", "Rejected", "FACEBOOK_APP_SECRET is not set in Admin.")
		http.Error(w, "not configured", 503)
		return
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), []byte(r.Header.Get("X-Hub-Signature-256"))) != 1 {
		s.hookLog(context.Background(), "", "Rejected", "Meta sent an event but the signature did not match. FACEBOOK_APP_SECRET in Admin must equal the App secret of the same Meta app.")
		http.Error(w, "bad signature", 403)
		return
	}
	w.WriteHeader(200)
	go s.crmMetaEvents(body)
}

func (s *server) crmMetaEvents(body []byte) {
	var ev struct {
		Object string `json:"object"`
		Entry  []struct {
			ID      string `json:"id"`
			Changes []struct {
				Field string          `json:"field"`
				Value json.RawMessage `json:"value"`
			} `json:"changes"`
			Messaging []struct {
				Sender  struct{ ID string } `json:"sender"`
				Message struct {
					Mid    string `json:"mid"`
					Text   string `json:"text"`
					IsEcho bool   `json:"is_echo"`
				} `json:"message"`
			} `json:"messaging"`
		} `json:"entry"`
	}
	if json.Unmarshal(body, &ev) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	if ev.Object == "whatsapp_business_account" {
		s.waEvents(body)
		return
	}
	ig := ev.Object == "instagram"
	for _, en := range ev.Entry {
		wid, tok, err := s.crmPageToken(ctx, en.ID)
		if err != nil || wid == "" {
			why := "no Page or Instagram account with this ID is connected to a CRM workspace. Press Connect my Pages under Lead sources."
			if err != nil && wid != "" {
				why = "connected, but its access token failed: " + trunc(err.Error(), 150) + " Reconnect Facebook under Connected social."
			}
			s.hookLog(ctx, wid, ev.Object+" event", "Received for ID "+en.ID+" but "+why)
			continue
		}
		s.hookLog(ctx, wid, ev.Object+" event", fmt.Sprintf("Received: %d change(s), %d message(s)", len(en.Changes), len(en.Messaging)))
		for _, ch := range en.Changes {
			switch ch.Field {
			case "leadgen":
				var v struct {
					LeadgenID string `json:"leadgen_id"`
					FormID    string `json:"form_id"`
				}
				json.Unmarshal(ch.Value, &v)
				var l metaLead
				if v.LeadgenID != "" && getJSON(ctx, "https://graph.facebook.com/v21.0/"+v.LeadgenID+"?"+url.Values{"fields": {"id,created_time,field_data,ad_name,campaign_name,form_id"}, "access_token": {tok}}.Encode(), &l) == nil {
					s.crmAddLeadMeta(ctx, wid, l, "")
				}
			case "feed", "comments":
				var v struct {
					Item      string `json:"item"`
					Verb      string `json:"verb"`
					CommentID string `json:"comment_id"`
					ID        string `json:"id"`
					PostID    string `json:"post_id"`
					Media     struct {
						ID string `json:"id"`
					} `json:"media"`
					Message string `json:"message"`
					Text    string `json:"text"`
					From    struct {
						ID       string `json:"id"`
						Name     string `json:"name"`
						Username string `json:"username"`
					} `json:"from"`
				}
				json.Unmarshal(ch.Value, &v)
				if (ch.Field == "feed" && (v.Item != "comment" || v.Verb != "add")) || v.From.ID == "" || v.From.ID == en.ID {
					continue
				}
				kind := ifs(ig, "ig_comment", "fb_comment")
				f := map[string]any{"name": clip(firstNonEmptyStr(v.From.Name, v.From.Username, "Commenter"), 120), "source": kind, "sourceRef": kind + ":" + v.From.ID}
				ctxt := firstNonEmptyStr(v.Message, v.Text)
				lid := s.crmInboundID(ctx, wid, f, "Comment: "+ctext(ctxt, 500))
				s.crmFlow(ctx, flowEvent{wid: wid, tok: tok, pageID: en.ID, ig: ig, sender: v.From.ID, name: firstNonEmptyStr(v.From.Name, v.From.Username), text: ctxt,
					commentID: firstNonEmptyStr(v.CommentID, ifs(ig, v.ID, "")), postID: firstNonEmptyStr(v.PostID, v.Media.ID), leadID: lid})
			}
		}
		for _, m := range en.Messaging {
			if m.Message.IsEcho || m.Sender.ID == "" || m.Sender.ID == en.ID || m.Message.Text == "" {
				continue
			}
			kind := ifs(ig, "ig_dm", "fb_dm")
			name := "Message from " + kind
			var p struct {
				Name     string `json:"name"`
				Username string `json:"username"`
			}
			if getJSON(ctx, "https://graph.facebook.com/v21.0/"+m.Sender.ID+"?"+url.Values{"fields": {ifs(ig, "username", "name")}, "access_token": {tok}}.Encode(), &p) == nil {
				name = firstNonEmptyStr(p.Name, p.Username, name)
			}
			f := map[string]any{"name": clip(name, 120), "source": kind, "sourceRef": kind + ":" + m.Sender.ID}
			lid := s.crmInboundID(ctx, wid, f, "Message: "+ctext(m.Message.Text, 500))
			s.crmFlow(ctx, flowEvent{wid: wid, tok: tok, pageID: en.ID, ig: ig, sender: m.Sender.ID, name: name, text: m.Message.Text, mid: m.Message.Mid, leadID: lid})
		}
	}
}

// Ad spend vs CRM revenue: cost per lead and ROAS for Meta lead ads.
func (s *server) crmAds(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("reports", 'v') {
		c.deny(w, "view reports")
		return
	}
	days := 30
	preset := "last_30d"
	if r.URL.Query().Get("days") == "90" {
		days, preset = 90, "last_90d"
	}
	owner := firstNonEmptyStr(sv(c.ws, "metaOwner"), c.uid)
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	accts, tok, err := s.adsAccounts(ctx, owner)
	if err != nil || len(accts) == 0 {
		jsonOut(w, map[string]any{"connected": false})
		return
	}
	acct := strings.TrimPrefix(accts[0].ID, "act_")
	rows, err := fetchPerf(ctx, tok, acct, "campaign", preset)
	if err != nil {
		jsonOut(w, map[string]any{"connected": false})
		return
	}
	spend := 0.0
	for _, x := range rows {
		spend += atof(x.Spend)
	}
	from := time.Now().AddDate(0, 0, -days)
	leads, revenue, won := 0, 0.0, 0
	for _, l := range c.list("leads") {
		if t, _ := l["createdAt"].(time.Time); sv(l, "source") == "meta_lead_ad" && t.After(from) {
			leads++
		}
	}
	for _, d := range c.list("deals") {
		cl, _ := d["closedAt"].(time.Time)
		if sv(d, "stage") == "won" && sv(d, "source") == "meta_lead_ad" && cl.After(from) {
			revenue += nv(d, "value")
			won++
		}
	}
	cpl, roas := 0.0, 0.0
	if leads > 0 {
		cpl = spend / float64(leads)
	}
	if spend > 0 {
		roas = revenue / spend
	}
	jsonOut(w, map[string]any{"connected": true, "days": days, "adCurrency": accts[0].Currency, "spend": spend, "leads": leads, "cpl": cpl, "won": won, "revenue": revenue, "roas": roas})
}
