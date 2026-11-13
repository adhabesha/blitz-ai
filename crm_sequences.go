package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

var farFuture = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------- helpers

func supKey(wid, email string) string { return hid(wid, strings.ToLower(email)) }

func (s *server) suppressed(ctx context.Context, wid, email string) bool {
	_, err := s.db.Collection("crm_suppress").Doc(supKey(wid, email)).Get(ctx)
	return err == nil
}

func (s *server) suppress(ctx context.Context, wid, email, why string) {
	s.db.Collection("crm_suppress").Doc(supKey(wid, email)).Set(ctx, map[string]any{"wid": wid, "email": strings.ToLower(email), "reason": why, "at": time.Now()})
}

func tplRender(t string, vars map[string]string) string {
	for k, v := range vars {
		t = strings.ReplaceAll(t, "{{"+k+"}}", v)
	}
	return t
}

func firstWord(n string) string {
	if f := strings.Fields(n); len(f) > 0 && !strings.HasPrefix(n, "Team inbox") {
		return f[0]
	}
	return "there"
}

func (s *server) leadByEmail(ctx context.Context, wid, email string) (string, map[string]any) {
	it := s.crmWS(wid).Collection("leads").Where("email", "==", strings.ToLower(email)).Limit(1).Documents(ctx)
	defer it.Stop()
	d, err := it.Next()
	if err != nil {
		return "", nil
	}
	return d.Ref.ID, d.Data()
}

func (s *server) seqStat(ctx context.Context, wid, sid, key string) {
	if sid == "" {
		return
	}
	s.crmWS(wid).Collection("sequences").Doc(sid).Update(ctx, []firestore.Update{{Path: "stats." + key, Value: firestore.Increment(1)}})
}

func cleanSteps(raw []map[string]any) ([]map[string]any, string) {
	if len(raw) < 1 || len(raw) > 8 {
		return nil, "A sequence needs between 1 and 8 emails."
	}
	out := []map[string]any{}
	for i, st := range raw {
		sub, body := clip(sv(st, "subject"), 150), ctext(sv(st, "body"), 5000)
		if sub == "" || body == "" {
			return nil, fmt.Sprintf("Email %d needs a subject and a message.", i+1)
		}
		d := int(nv(st, "delayDays"))
		if d < 0 {
			d = 0
		}
		if d > 60 {
			d = 60
		}
		out = append(out, map[string]any{"delayDays": d, "subject": sub, "body": body})
	}
	return out, ""
}

// ---------------------------------------------------------------- sequence CRUD

func (s *server) crmSequences(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'v') {
		c.deny(w, "view sequences")
		return
	}
	it := s.crmWS(c.wid).Collection("sequences").Limit(100).Documents(c.ctx)
	defer it.Stop()
	out := []map[string]any{}
	for {
		d, err := it.Next()
		if err == iterator.Done || err != nil {
			break
		}
		x := d.Data()
		x["id"] = d.Ref.ID
		out = append(out, x)
	}
	jsonOut(w, map[string]any{"items": out})
}

func (s *server) outboundReady(ws map[string]any) string {
	o := obOf(ws)
	switch {
	case !mailConfigured():
		return "Email sending is not set up. Add MAIL_FROM and SMTP_HOST, SMTP_USER, SMTP_PASS in Admin (use your own mailbox for outreach)."
	case env("BASE_URL") == "":
		return "Add BASE_URL in Admin (your public app address). It is needed for the unsubscribe link."
	case strings.TrimSpace(o.Address) == "":
		return "Add your company's postal address in Outbound settings. Anti-spam laws require it in every cold email."
	}
	return ""
}

func (s *server) crmSequenceSave(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'e') || !c.can("leads", 'c') {
		c.deny(w, "edit sequences")
		return
	}
	var in struct {
		ID           string           `json:"id"`
		Name         string           `json:"name"`
		Active       bool             `json:"active"`
		Steps        []map[string]any `json:"steps"`
		WindowStart  int              `json:"windowStart"`
		WindowEnd    int              `json:"windowEnd"`
		WeekdaysOnly bool             `json:"weekdaysOnly"`
		DailyLimit   int              `json:"dailyLimit"`
	}
	if !readBody(w, r, &in, 100<<10) {
		return
	}
	name := clip(in.Name, 80)
	if name == "" {
		http.Error(w, "Give the sequence a name.", 400)
		return
	}
	steps, msg := cleanSteps(in.Steps)
	if msg != "" {
		http.Error(w, msg, 400)
		return
	}
	if in.WindowStart < 0 || in.WindowStart > 23 {
		in.WindowStart = 9
	}
	if in.WindowEnd <= in.WindowStart || in.WindowEnd > 24 {
		in.WindowEnd = 17
	}
	if in.DailyLimit < 1 {
		in.DailyLimit = 30
	}
	if in.DailyLimit > 200 {
		in.DailyLimit = 200
	}
	if in.Active {
		if m := s.outboundReady(c.ws); m != "" {
			http.Error(w, m, 400)
			return
		}
	}
	col := s.crmWS(c.wid).Collection("sequences")
	f := map[string]any{"name": name, "active": in.Active, "steps": steps, "windowStart": in.WindowStart, "windowEnd": in.WindowEnd, "weekdaysOnly": in.WeekdaysOnly, "dailyLimit": in.DailyLimit, "updatedAt": time.Now()}
	id := in.ID
	if id == "" {
		ref := col.NewDoc()
		f["createdAt"], f["createdBy"], f["stats"] = time.Now(), c.uid, map[string]any{}
		if _, err := ref.Set(c.ctx, f); err != nil {
			http.Error(w, "Could not save.", 500)
			return
		}
		id = ref.ID
	} else {
		if strings.Contains(id, "/") {
			http.Error(w, "bad request", 400)
			return
		}
		if _, err := col.Doc(id).Set(c.ctx, f, firestoreMerge()); err != nil {
			http.Error(w, "Could not save.", 500)
			return
		}
	}
	jsonOut(w, map[string]any{"id": id})
}

func (s *server) crmSequenceDelete(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'd') {
		c.deny(w, "delete sequences")
		return
	}
	var in struct{ ID string }
	if !readBody(w, r, &in, 2<<10) || in.ID == "" || strings.Contains(in.ID, "/") {
		return
	}
	it := s.db.Collection("crm_enroll").Where("wid", "==", c.wid).Where("seqId", "==", in.ID).Limit(500).Documents(c.ctx)
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		d.Ref.Delete(c.ctx)
	}
	it.Stop()
	s.crmWS(c.wid).Collection("sequences").Doc(in.ID).Delete(c.ctx)
	jsonOut(w, map[string]bool{"ok": true})
}

func (s *server) crmSequenceEnroll(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'e') {
		c.deny(w, "start sequences")
		return
	}
	var in struct {
		SeqID   string   `json:"seqId"`
		LeadIDs []string `json:"leadIds"`
	}
	if !readBody(w, r, &in, 50<<10) {
		return
	}
	if in.SeqID == "" || strings.Contains(in.SeqID, "/") || len(in.LeadIDs) == 0 || len(in.LeadIDs) > 200 {
		http.Error(w, "Pick a sequence and 1 to 200 leads.", 400)
		return
	}
	sd, err := s.crmWS(c.wid).Collection("sequences").Doc(in.SeqID).Get(c.ctx)
	if err != nil {
		http.Error(w, "Sequence not found.", 404)
		return
	}
	steps, _ := sd.Data()["steps"].([]any)
	if len(steps) == 0 {
		http.Error(w, "That sequence has no emails.", 400)
		return
	}
	first, _ := steps[0].(map[string]any)
	start := time.Now().Add(time.Duration(nv(first, "delayDays")) * 24 * time.Hour)
	added, skipped, noMail := 0, 0, 0
	for _, lid := range in.LeadIDs {
		if lid == "" || strings.Contains(lid, "/") {
			continue
		}
		ld, err := s.crmWS(c.wid).Collection("leads").Doc(lid).Get(c.ctx)
		if err != nil || !c.sees("leads", ld.Data()) {
			continue
		}
		x := ld.Data()
		em := strings.ToLower(sv(x, "email"))
		if !reEmailOK.MatchString(em) {
			noMail++
			continue
		}
		if s.suppressed(c.ctx, c.wid, em) {
			skipped++
			continue
		}
		id := hid(in.SeqID, lid)
		_, err = s.db.Collection("crm_enroll").Doc(id).Create(c.ctx, map[string]any{"wid": c.wid, "seqId": in.SeqID, "leadId": lid, "email": em, "name": sv(x, "name"), "company": sv(x, "company"), "title": sv(x, "title"),
			"step": 0, "nextAt": start, "status": "active", "opens": 0, "attempts": 0, "createdAt": time.Now(), "by": c.uid})
		if err != nil {
			skipped++
			continue
		}
		added++
		s.seqStat(c.ctx, c.wid, in.SeqID, "enrolled")
	}
	jsonOut(w, map[string]int{"added": added, "skipped": skipped, "noEmail": noMail})
}

func (s *server) crmEnrollments(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'v') {
		c.deny(w, "view sequences")
		return
	}
	sid := r.URL.Query().Get("seqId")
	it := s.db.Collection("crm_enroll").Where("wid", "==", c.wid).Where("seqId", "==", sid).Limit(300).Documents(c.ctx)
	defer it.Stop()
	out := []map[string]any{}
	for {
		d, err := it.Next()
		if err == iterator.Done || err != nil {
			break
		}
		x := d.Data()
		na := timeOf(x["nextAt"])
		var nx any
		if !na.IsZero() && na.Before(farFuture.Add(-time.Hour)) {
			nx = na
		}
		out = append(out, map[string]any{"id": d.Ref.ID, "name": sv(x, "name"), "email": sv(x, "email"), "company": sv(x, "company"), "status": sv(x, "status"),
			"step": nv(x, "step"), "nextAt": nx, "opens": nv(x, "opens"), "lastErr": sv(x, "lastErr"), "leadId": sv(x, "leadId")})
	}
	jsonOut(w, map[string]any{"items": out})
}

func (s *server) crmEnrollStop(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'e') {
		c.deny(w, "stop sequences")
		return
	}
	var in struct{ ID string }
	if !readBody(w, r, &in, 2<<10) || in.ID == "" || strings.Contains(in.ID, "/") {
		return
	}
	ref := s.db.Collection("crm_enroll").Doc(in.ID)
	d, err := ref.Get(c.ctx)
	if err != nil || sv(d.Data(), "wid") != c.wid {
		http.Error(w, "Not found", 404)
		return
	}
	ref.Set(c.ctx, map[string]any{"status": "stopped", "nextAt": farFuture}, firestoreMerge())
	jsonOut(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------- scheduler

var obMu sync.Mutex

func (s *server) outboundLoop() {
	time.Sleep(45 * time.Second)
	for {
		func() {
			defer func() {
				if e := recover(); e != nil {
					log.Println("outbound loop panic:", e)
				}
			}()
			s.outboundTick()
		}()
		time.Sleep(3 * time.Minute)
	}
}

func inWindow(now time.Time, tz, from, to int, weekdays bool) bool {
	l := now.UTC().Add(time.Duration(tz) * time.Hour)
	if weekdays && (l.Weekday() == time.Saturday || l.Weekday() == time.Sunday) {
		return false
	}
	return l.Hour() >= from && l.Hour() < to
}

func (s *server) outboundTick() {
	if !obMu.TryLock() {
		return
	}
	defer obMu.Unlock()
	if s.db == nil || !mailConfigured() || env("BASE_URL") == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	now := time.Now()
	it := s.db.Collection("crm_enroll").Where("nextAt", "<=", now).OrderBy("nextAt", firestore.Asc).Limit(120).Documents(ctx)
	type job struct {
		ref *firestore.DocumentRef
		x   map[string]any
	}
	var jobs []job
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		x := d.Data()
		if sv(x, "status") == "active" {
			jobs = append(jobs, job{d.Ref, x})
		}
	}
	it.Stop()
	seqs := map[string]map[string]any{}
	wss := map[string]map[string]any{}
	sentToday := map[string]int{}
	sent := 0
	for _, j := range jobs {
		if sent >= 20 || ctx.Err() != nil {
			break
		}
		wid, sid := sv(j.x, "wid"), sv(j.x, "seqId")
		sk := wid + "/" + sid
		sq, ok := seqs[sk]
		if !ok {
			if d, err := s.crmWS(wid).Collection("sequences").Doc(sid).Get(ctx); err == nil {
				sq = d.Data()
			}
			seqs[sk] = sq
		}
		if sq == nil {
			j.ref.Set(ctx, map[string]any{"status": "stopped", "nextAt": farFuture}, firestoreMerge())
			continue
		}
		if b, _ := sq["active"].(bool); !b {
			continue
		}
		ws, ok := wss[wid]
		if !ok {
			if d, err := s.crmWS(wid).Get(ctx); err == nil {
				ws = d.Data()
			}
			wss[wid] = ws
		}
		o := obOf(ws)
		if s.outboundReady(ws) != "" {
			continue
		}
		wb, _ := sq["weekdaysOnly"].(bool)
		if !inWindow(now, o.Tz, int(nv(sq, "windowStart")), int(nv(sq, "windowEnd")), wb) {
			continue
		}
		day := now.UTC().Add(time.Duration(o.Tz) * time.Hour).Format("2006-01-02")
		cnt, seen := sentToday[sk]
		if !seen {
			if sv(sq, "dayKey") == day {
				cnt = int(nv(sq, "dayCount"))
			}
		}
		lim := int(nv(sq, "dailyLimit"))
		if lim < 1 {
			lim = 30
		}
		if cnt >= lim {
			sentToday[sk] = cnt
			continue
		}
		em := sv(j.x, "email")
		if s.suppressed(ctx, wid, em) {
			j.ref.Set(ctx, map[string]any{"status": "unsubscribed", "nextAt": farFuture}, firestoreMerge())
			continue
		}
		steps, _ := sq["steps"].([]any)
		idx := int(nv(j.x, "step"))
		if idx >= len(steps) {
			j.ref.Set(ctx, map[string]any{"status": "done", "nextAt": farFuture}, firestoreMerge())
			continue
		}
		// claim it so a second pass never sends it twice
		j.ref.Set(ctx, map[string]any{"nextAt": now.Add(30 * time.Minute)}, firestoreMerge())
		st, _ := steps[idx].(map[string]any)
		err := s.sendSeqMail(ctx, j.ref.ID, j.x, st, o)
		if err != nil {
			at := int(nv(j.x, "attempts")) + 1
			up := map[string]any{"attempts": at, "lastErr": trunc(err.Error(), 200), "nextAt": now.Add(time.Hour)}
			if at >= 3 {
				up["status"], up["nextAt"] = "error", farFuture
			}
			j.ref.Set(ctx, up, firestoreMerge())
			continue
		}
		sent++
		cnt++
		sentToday[sk] = cnt
		s.crmWS(wid).Collection("sequences").Doc(sid).Set(ctx, map[string]any{"dayKey": day, "dayCount": cnt}, firestoreMerge())
		s.seqStat(ctx, wid, sid, "sent")
		up := map[string]any{"step": idx + 1, "attempts": 0, "lastErr": "", "sentAt": time.Now()}
		if idx+1 >= len(steps) {
			up["status"], up["nextAt"] = "done", farFuture
		} else {
			ns, _ := steps[idx+1].(map[string]any)
			d := time.Duration(nv(ns, "delayDays")) * 24 * time.Hour
			if d < time.Hour {
				d = time.Hour
			}
			up["nextAt"] = time.Now().Add(d)
		}
		j.ref.Set(ctx, up, firestoreMerge())
		if lid := sv(j.x, "leadId"); lid != "" {
			lr := s.crmWS(wid).Collection("leads").Doc(lid)
			if ld, err := lr.Get(ctx); err == nil {
				if sv(ld.Data(), "status") == "new" {
					lr.Set(ctx, map[string]any{"status": "contacted", "updatedAt": time.Now()}, firestoreMerge())
				}
				s.crmLog(ctx, wid, "leads", lid, sv(ld.Data(), "name"), "email", fmt.Sprintf("Sequence email %d sent: %s", idx+1, sv(st, "subject")), "", sv(ld.Data(), "ownerUid"))
			}
		}
		time.Sleep(4 * time.Second)
	}
}

func textToHTML(t string) string {
	ps := strings.Split(html.EscapeString(strings.TrimSpace(t)), "\n\n")
	for i, p := range ps {
		ps[i] = "<p style=\"margin:0 0 12px\">" + strings.ReplaceAll(strings.TrimSpace(p), "\n", "<br>") + "</p>"
	}
	return strings.Join(ps, "")
}

func (s *server) sendSeqMail(ctx context.Context, token string, en, st map[string]any, o obSettings) error {
	base := strings.TrimRight(env("BASE_URL"), "/")
	vars := map[string]string{"first_name": firstWord(sv(en, "name")), "name": sv(en, "name"), "company": firstNonEmptyStr(sv(en, "company"), "your company"), "title": sv(en, "title"), "sender": firstNonEmptyStr(o.FromName, o.Company)}
	subj := tplRender(sv(st, "subject"), vars)
	body := tplRender(sv(st, "body"), vars)
	unsub := base + "/u/" + token
	foot := "\n\n--\nNot interested? Unsubscribe: " + unsub + "\n" + strings.TrimSpace(firstNonEmptyStr(o.Company, "")+" "+o.Address)
	txt := body + foot
	htm := `<div style="font:14px/1.55 Arial,sans-serif;color:#222">` + textToHTML(body) +
		`<p style="margin:18px 0 0;font-size:11px;color:#888">Not interested? <a href="` + html.EscapeString(unsub) + `" style="color:#888">Unsubscribe</a><br>` + html.EscapeString(strings.TrimSpace(firstNonEmptyStr(o.Company, "")+" "+o.Address)) + `</p>` +
		`<img src="` + base + "/t/o/" + token + `.gif" width="1" height="1" alt="" style="display:none"></div>`
	hdr := map[string]string{"List-Unsubscribe": "<" + unsub + ">", "List-Unsubscribe-Post": "List-Unsubscribe=One-Click"}
	if o.ReplyTo != "" {
		hdr["Reply-To"] = o.ReplyTo
	}
	c2, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	return sendMailH(c2, sv(en, "email"), subj, htm, txt, hdr, true)
}

// ---------------------------------------------------------------- public tracking + unsubscribe

var gif1 = []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b}

var reToken = regexp.MustCompile(`^[0-9a-f]{20}$`)

func (s *server) trackOpen(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	tok := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/t/o/"), ".gif")
	if reToken.MatchString(tok) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		ref := s.db.Collection("crm_enroll").Doc(tok)
		if d, err := ref.Get(ctx); err == nil {
			first := nv(d.Data(), "opens") == 0
			ref.Update(ctx, []firestore.Update{{Path: "opens", Value: firestore.Increment(1)}})
			if first {
				s.seqStat(ctx, sv(d.Data(), "wid"), sv(d.Data(), "seqId"), "opens")
			}
		}
	}
	w.Write(gif1)
}

func (s *server) unsubscribe(w http.ResponseWriter, r *http.Request) {
	tok := strings.TrimPrefix(r.URL.Path, "/u/")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page := func(t, body string) {
		fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><title>%s</title><body style="font-family:Arial,sans-serif;max-width:460px;margin:12vh auto;padding:0 20px;color:#222"><h2>%s</h2>%s</body>`, t, t, body)
	}
	if !reToken.MatchString(tok) {
		page("Link not valid", "<p>This unsubscribe link is not valid.</p>")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	ref := s.db.Collection("crm_enroll").Doc(tok)
	d, err := ref.Get(ctx)
	if err != nil {
		page("Link not valid", "<p>This unsubscribe link is not valid or has expired.</p>")
		return
	}
	x := d.Data()
	if r.Method != http.MethodPost {
		page("Unsubscribe", `<p>Stop receiving emails from this sender at <b>`+html.EscapeString(sv(x, "email"))+`</b>?</p><form method=post><button style="padding:10px 22px;font-size:15px;border:0;border-radius:8px;background:#2563eb;color:#fff;cursor:pointer">Yes, unsubscribe me</button></form>`)
		return
	}
	wid, em := sv(x, "wid"), sv(x, "email")
	s.suppress(ctx, wid, em, "unsubscribed")
	s.stopAllFor(ctx, wid, em, "unsubscribed", "")
	if lid, ld := s.leadByEmail(ctx, wid, em); lid != "" {
		s.crmLog(ctx, wid, "leads", lid, sv(ld, "name"), "note", "Unsubscribed from email outreach", "", sv(ld, "ownerUid"))
	}
	s.seqStat(ctx, wid, sv(x, "seqId"), "unsub")
	page("You are unsubscribed", "<p>You will not receive any more emails from this sender.</p>")
}

// stopAllFor ends every running sequence for one address in a workspace.
func (s *server) stopAllFor(ctx context.Context, wid, email, status, onlySeq string) int {
	it := s.db.Collection("crm_enroll").Where("wid", "==", wid).Where("email", "==", strings.ToLower(email)).Limit(50).Documents(ctx)
	defer it.Stop()
	n := 0
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		if sv(d.Data(), "status") == "active" {
			d.Ref.Set(ctx, map[string]any{"status": status, "nextAt": farFuture}, firestoreMerge())
			s.seqStat(ctx, wid, sv(d.Data(), "seqId"), ifs(status == "replied", "replies", "stopped"))
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- smart inbox

var inboxLabels = []string{"interested", "follow_up", "not_interested", "out_of_office", "unsubscribe", "bounce", "other"}

var reQuoteCut = regexp.MustCompile(`(?im)^(on .{5,120} wrote:|-{2,}\s*original message|from:\s.+@.+)`)

func replyOnly(t string) string {
	t = strings.ReplaceAll(t, "\r", "")
	var keep []string
	for _, l := range strings.Split(t, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), ">") {
			continue
		}
		keep = append(keep, l)
	}
	t = strings.Join(keep, "\n")
	if loc := reQuoteCut.FindStringIndex(t); loc != nil && loc[0] > 10 {
		t = t[:loc[0]]
	}
	return ctext(t, 4000)
}

func heuristicLabel(from, subj, text string) string {
	all := strings.ToLower(subj + " " + text)
	fl := strings.ToLower(from)
	switch {
	case strings.Contains(fl, "mailer-daemon") || strings.Contains(fl, "postmaster") || strings.Contains(all, "undeliverable") || strings.Contains(all, "delivery status notification") || strings.Contains(all, "address not found"):
		return "bounce"
	case strings.Contains(all, "out of office") || strings.Contains(all, "automatic reply") || strings.Contains(all, "auto-reply") || strings.Contains(all, "on vacation") || strings.Contains(all, "away from the office"):
		return "out_of_office"
	case strings.Contains(all, "unsubscribe") || strings.Contains(all, "remove me") || strings.Contains(all, "stop emailing") || strings.Contains(all, "take me off"):
		return "unsubscribe"
	case strings.Contains(all, "not interested") || strings.Contains(all, "no thanks") || strings.Contains(all, "no thank you") || strings.Contains(all, "not a fit") || strings.Contains(all, "don't contact") || strings.Contains(all, "do not contact"):
		return "not_interested"
	case strings.Contains(all, "interested") || strings.Contains(all, "let's talk") || strings.Contains(all, "lets talk") || strings.Contains(all, "book a call") || strings.Contains(all, "send me more") || strings.Contains(all, "pricing") || strings.Contains(all, "demo") || strings.Contains(all, "sounds good"):
		return "interested"
	case strings.Contains(all, "next quarter") || strings.Contains(all, "later") || strings.Contains(all, "follow up") || strings.Contains(all, "get back to you") || strings.Contains(all, "not right now") || strings.Contains(all, "busy"):
		return "follow_up"
	}
	return "other"
}

func (s *server) classifyReply(ctx context.Context, o obSettings, from, subj, text string) (label, summary, suggest string) {
	label = heuristicLabel(from, subj, text)
	if label == "bounce" || label == "out_of_office" || env("GEMINI_API_KEY") == "" {
		return label, trunc(strings.ReplaceAll(text, "\n", " "), 140), ""
	}
	prompt := fmt.Sprintf(`You triage replies to B2B cold emails. The sender sells: %s
Classify the reply into exactly one label: interested (wants to talk, asks for info, price or a meeting), follow_up (not now, later, needs time, or asks to be contacted later), not_interested (declines), out_of_office (automatic away reply), unsubscribe (asks to stop or be removed), bounce (delivery failure), other.
Also write "summary" (max 20 words, what they said) and "suggested_reply" (a short, friendly plain-text reply under 70 words that moves it forward; empty for not_interested, unsubscribe, bounce, out_of_office).
Reply subject: %s
Reply text: %s
Return JSON only: {"label":"","summary":"","suggested_reply":""}`, clip(o.Offer, 500), clip(subj, 150), ctext(text, 1800))
	cc, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	out, err := s.gemini(cc, prompt)
	if err != nil {
		return label, trunc(strings.ReplaceAll(text, "\n", " "), 140), ""
	}
	var r struct{ Label, Summary, SuggestedReply string }
	r.Label = ""
	var m map[string]string
	if json.Unmarshal([]byte(cleanJSON(out)), &m) == nil {
		r.Label, r.Summary, r.SuggestedReply = strings.ToLower(m["label"]), m["summary"], m["suggested_reply"]
	}
	if inList(inboxLabels, r.Label) {
		label = r.Label
	}
	return label, clip(firstNonEmptyStr(r.Summary, text), 200), ctext(r.SuggestedReply, 800)
}

// applyReply updates the lead and the running sequences after a reply has a label.
func (s *server) applyReply(ctx context.Context, wid, email, label, summary string, firstTime bool) (leadID, leadName string) {
	em := strings.ToLower(email)
	switch label {
	case "bounce":
		s.suppress(ctx, wid, em, "bounced")
		s.stopAllFor(ctx, wid, em, "bounced", "")
	case "unsubscribe":
		s.suppress(ctx, wid, em, "asked to stop")
		s.stopAllFor(ctx, wid, em, "unsubscribed", "")
	case "out_of_office":
		// keep the sequence running
	default:
		if firstTime {
			s.stopAllFor(ctx, wid, em, "replied", "")
		} else {
			s.stopAllFor(ctx, wid, em, "replied", "")
		}
	}
	lid, ld := s.leadByEmail(ctx, wid, em)
	if lid == "" {
		return "", ""
	}
	st := sv(ld, "status")
	up := map[string]any{"updatedAt": time.Now()}
	switch {
	case label == "interested" && (st == "new" || st == "contacted"):
		up["status"] = "qualified"
	case (label == "not_interested" || label == "unsubscribe") && (st == "new" || st == "contacted"):
		up["status"] = "unqualified"
	}
	if len(up) > 1 {
		s.crmWS(wid).Collection("leads").Doc(lid).Set(ctx, up, firestoreMerge())
	}
	if firstTime {
		s.crmLog(ctx, wid, "leads", lid, sv(ld, "name"), "email", "Replied ("+strings.ReplaceAll(label, "_", " ")+"): "+summary, "", sv(ld, "ownerUid"))
		if label == "interested" || label == "follow_up" {
			s.crmWS(wid).Collection("tasks").NewDoc().Set(ctx, map[string]any{"title": "Reply to " + sv(ld, "name") + " (" + strings.ReplaceAll(label, "_", " ") + ")", "due": time.Now().Add(24 * time.Hour).Format("2006-01-02"),
				"done": false, "priority": ifs(label == "interested", "high", "normal"), "assigneeUid": sv(ld, "ownerUid"), "relType": "leads", "relId": lid, "relName": sv(ld, "name"), "createdAt": time.Now(), "updatedAt": time.Now()})
		}
	}
	return lid, sv(ld, "name")
}

func (s *server) inboxIngest(ctx context.Context, wid, from, subj, text, src string) (string, string, error) {
	addr, name := strings.ToLower(strings.TrimSpace(from)), ""
	if a, err := mail.ParseAddress(from); err == nil {
		addr, name = strings.ToLower(a.Address), a.Name
	} else if m := reEmailFind.FindString(from); m != "" {
		addr = strings.ToLower(m)
	}
	if !reEmailOK.MatchString(addr) {
		return "", "", fmt.Errorf("could not read the sender address")
	}
	text = replyOnly(text)
	wd, _ := s.crmWS(wid).Get(ctx)
	var ws map[string]any
	if wd != nil {
		ws = wd.Data()
	}
	label, summary, suggest := s.classifyReply(ctx, obOf(ws), addr, subj, text)
	lid, lname := s.applyReply(ctx, wid, addr, label, summary, true)
	ref := s.crmWS(wid).Collection("inbox").NewDoc()
	_, err := ref.Set(ctx, map[string]any{"from": addr, "fromName": firstNonEmptyStr(name, lname), "subject": clip(subj, 200), "text": text, "label": label, "summary": summary, "suggest": suggest,
		"leadId": lid, "leadName": lname, "handled": false, "source": src, "receivedAt": time.Now()})
	return ref.ID, label, err
}

func pick(m map[string]any, keys ...string) string {
	low := map[string]any{}
	for k, v := range m {
		low[strings.ToLower(k)] = v
	}
	for _, k := range keys {
		switch v := low[k].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return v
			}
		case map[string]any:
			if t := pick(v, "address", "email", "text"); t != "" {
				return t
			}
		}
	}
	if d, ok := low["data"].(map[string]any); ok {
		return pick(d, keys...)
	}
	return ""
}

// crmMailInbound receives forwarded replies: Mailgun routes, Postmark, SendGrid, Zapier/Make, Resend and similar.
func (s *server) crmMailInbound(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/crm/inbound/"), "/"), "/")
	if r.Method != http.MethodPost || len(parts) != 2 || parts[0] == "" || strings.Contains(parts[0], "/") {
		http.Error(w, "not found", 404)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	wd, err := s.crmWS(parts[0]).Get(ctx)
	sec := ""
	if err == nil {
		sec = sv(wd.Data(), "inboundSecret")
	}
	if sec == "" || subtle.ConstantTimeCompare([]byte(sec), []byte(parts[1])) != 1 {
		http.Error(w, "not found", 404)
		return
	}
	raw, _ := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	m := map[string]any{}
	if json.Unmarshal(raw, &m) != nil {
		m = map[string]any{}
		if q, e := url.ParseQuery(string(raw)); e == nil {
			for k, v := range q {
				if len(v) > 0 {
					m[k] = v[0]
				}
			}
		}
	}
	from := pick(m, "from", "sender", "from_email", "fromemail", "from_address", "reply_to")
	subj := pick(m, "subject")
	text := pick(m, "stripped-text", "text", "textbody", "body-plain", "body", "plain", "content", "snippet", "html", "htmlbody")
	if strings.Contains(text, "<") && strings.Contains(text, ">") {
		text = html.UnescapeString(regexp.MustCompile(`(?s)<style.*?</style>|<[^>]+>`).ReplaceAllString(text, " "))
	}
	if from == "" || (subj == "" && text == "") {
		http.Error(w, "missing from, subject or text", 400)
		return
	}
	id, label, err := s.inboxIngest(ctx, parts[0], from, subj, text, "forwarded")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	jsonOut(w, map[string]string{"id": id, "label": label})
}

func (s *server) crmInbox(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'v') {
		c.deny(w, "view the inbox")
		return
	}
	it := s.crmWS(c.wid).Collection("inbox").OrderBy("receivedAt", firestore.Desc).Limit(200).Documents(c.ctx)
	defer it.Stop()
	out := []map[string]any{}
	for {
		d, err := it.Next()
		if err == iterator.Done || err != nil {
			break
		}
		x := d.Data()
		x["id"] = d.Ref.ID
		out = append(out, x)
	}
	jsonOut(w, map[string]any{"items": out})
}

func (s *server) crmInboxAdd(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'c') {
		c.deny(w, "add replies")
		return
	}
	var in struct{ From, Subject, Text string }
	if !readBody(w, r, &in, 60<<10) {
		return
	}
	if strings.TrimSpace(in.Text) == "" || strings.TrimSpace(in.From) == "" {
		http.Error(w, "Enter who it is from and what they wrote.", 400)
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 50*time.Second)
	defer cancel()
	id, label, err := s.inboxIngest(ctx, c.wid, in.From, in.Subject, in.Text, "pasted")
	if err != nil {
		http.Error(w, "That sender address is not valid.", 400)
		return
	}
	jsonOut(w, map[string]string{"id": id, "label": label})
}

func (s *server) crmInboxHandle(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'e') {
		c.deny(w, "update the inbox")
		return
	}
	var in struct {
		ID      string
		Label   string
		Handled *bool
		Delete  bool
	}
	if !readBody(w, r, &in, 4<<10) || in.ID == "" || strings.Contains(in.ID, "/") {
		return
	}
	ref := s.crmWS(c.wid).Collection("inbox").Doc(in.ID)
	d, err := ref.Get(c.ctx)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	if in.Delete {
		ref.Delete(c.ctx)
		jsonOut(w, map[string]bool{"ok": true})
		return
	}
	up := map[string]any{}
	if in.Handled != nil {
		up["handled"] = *in.Handled
	}
	if in.Label != "" && inList(inboxLabels, in.Label) && in.Label != sv(d.Data(), "label") {
		up["label"] = in.Label
		s.applyReply(c.ctx, c.wid, sv(d.Data(), "from"), in.Label, sv(d.Data(), "summary"), false)
	}
	if len(up) > 0 {
		ref.Set(c.ctx, up, firestoreMerge())
	}
	jsonOut(w, map[string]bool{"ok": true})
}

func (s *server) crmInboxReply(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'e') {
		c.deny(w, "send replies")
		return
	}
	var in struct{ ID, Subject, Body string }
	if !readBody(w, r, &in, 30<<10) || in.ID == "" || strings.Contains(in.ID, "/") || strings.TrimSpace(in.Body) == "" {
		http.Error(w, "Write a message first.", 400)
		return
	}
	ref := s.crmWS(c.wid).Collection("inbox").Doc(in.ID)
	d, err := ref.Get(c.ctx)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	x := d.Data()
	if s.suppressed(c.ctx, c.wid, sv(x, "from")) {
		http.Error(w, "This person unsubscribed or the address bounced, so no email can be sent to it.", 400)
		return
	}
	o := obOf(c.ws)
	hdr := map[string]string{}
	if o.ReplyTo != "" {
		hdr["Reply-To"] = o.ReplyTo
	}
	subj := firstNonEmptyStr(clip(in.Subject, 150), "Re: "+sv(x, "subject"))
	body := ctext(in.Body, 5000)
	ctx, cancel := context.WithTimeout(c.ctx, 45*time.Second)
	defer cancel()
	if err := sendMailH(ctx, sv(x, "from"), subj, `<div style="font:14px/1.55 Arial,sans-serif;color:#222">`+textToHTML(body)+`</div>`, body, hdr, true); err != nil {
		http.Error(w, "Could not send: "+trunc(err.Error(), 160), 502)
		return
	}
	ref.Set(c.ctx, map[string]any{"handled": true, "repliedAt": time.Now()}, firestoreMerge())
	if lid := sv(x, "leadId"); lid != "" {
		s.crmLog(c.ctx, c.wid, "leads", lid, sv(x, "leadName"), "email", "Replied by email: "+trunc(body, 300), c.uid, "")
	}
	jsonOut(w, map[string]bool{"ok": true})
}

// crmSequenceTest sends one email of a sequence to the signed-in person's own address so they can see it first.
func (s *server) crmSequenceTest(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'e') {
		c.deny(w, "send tests")
		return
	}
	var in struct{ Subject, Body string }
	if !readBody(w, r, &in, 30<<10) || strings.TrimSpace(in.Subject) == "" || strings.TrimSpace(in.Body) == "" {
		http.Error(w, "Write a subject and a message first.", 400)
		return
	}
	if m := s.outboundReady(c.ws); m != "" {
		http.Error(w, m, 400)
		return
	}
	if !reEmailOK.MatchString(c.email) {
		http.Error(w, "Your account has no email address.", 400)
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 50*time.Second)
	defer cancel()
	en := map[string]any{"email": c.email, "name": "Alex Sample", "company": "Sample Company", "title": "Managing Director"}
	st := map[string]any{"subject": "[TEST] " + in.Subject, "body": in.Body}
	if err := s.sendSeqMail(ctx, "00000000000000000000", en, st, obOf(c.ws)); err != nil {
		http.Error(w, "Could not send: "+trunc(err.Error(), 200), 502)
		return
	}
	jsonOut(w, map[string]string{"sentTo": c.email})
}
