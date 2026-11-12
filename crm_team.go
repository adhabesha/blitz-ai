package main

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

var reEmailOK = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

var crmRoleInfo = []map[string]any{
	{"key": "owner", "label": "Owner", "desc": "Everything, including the team and settings."},
	{"key": "admin", "label": "Administrator", "desc": "Everything except changing the owner. Sees all records."},
	{"key": "manager", "label": "Manager", "desc": "Full control of leads, deals, contacts and tasks for their department. Sees reports."},
	{"key": "marketing", "label": "Marketing", "desc": "Imports and exports leads, manages lead sources, views reports. Cannot edit deals."},
	{"key": "sales", "label": "Sales", "desc": "Works their own leads, deals, contacts and tasks. No delete, no reports."},
	{"key": "support", "label": "Support", "desc": "Edits contacts and companies and their tasks. Views leads and deals."},
	{"key": "viewer", "label": "Viewer", "desc": "Read-only access to records and reports."},
}

func (s *server) crmMe(w http.ResponseWriter, r *http.Request) {
	c, err := s.crmAuth(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	st, err := c.load()
	if err != nil {
		http.Error(w, "Could not load your workspace. Try again.", 500)
		return
	}
	out := map[string]any{"state": st, "email": c.email, "verified": c.verified}
	if st == "ok" {
		out["workspace"] = map[string]any{"id": c.wid, "name": sv(c.ws, "name"), "currency": firstNonEmptyStr(sv(c.ws, "currency"), "USD"), "autoAssign": c.ws["autoAssign"] == true}
		out["me"] = map[string]any{"uid": c.uid, "name": sv(c.m, "name"), "email": c.email, "role": c.role, "dept": c.dept, "scope": c.scope}
		out["perms"] = crmPerms[c.role]
		out["leadStatus"], out["dealStage"], out["stageProb"] = crmLeadStatus, crmDealStage, crmStageProb
		out["roles"] = crmRoleInfo
	}
	jsonOut(w, out)
}

func (s *server) crmCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	c, err := s.crmAuth(r)
	if err != nil || r.Method != http.MethodPost {
		http.Error(w, "unauthorized", 401)
		return
	}
	if st, _ := c.load(); st != "none" {
		http.Error(w, "You already belong to a workspace.", 400)
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	name := clip(in["name"], 80)
	if name == "" {
		http.Error(w, "Enter a company name.", 400)
		return
	}
	me := clip(in["me"], 60)
	if me == "" {
		me = c.email
		if i := strings.Index(me, "@"); i > 0 {
			me = me[:i]
		}
	}
	wid := randHex(8)
	ref := s.crmWS(wid)
	if _, err := ref.Set(c.ctx, map[string]any{"name": name, "ownerUid": c.uid, "currency": "USD", "hookSecret": randHex(12), "autoAssign": false, "createdAt": time.Now()}); err != nil {
		http.Error(w, "db error", 500)
		return
	}
	ref.Collection("members").Doc(c.uid).Set(c.ctx, map[string]any{"uid": c.uid, "email": c.email, "name": me, "role": "owner", "dept": "", "scope": "all", "status": "active", "joinedAt": time.Now()})
	s.db.Collection("users").Doc(c.uid).Collection("private").Doc("crm").Set(c.ctx, map[string]any{"wid": wid})
	jsonOut(w, map[string]string{"id": wid})
}

func (s *server) crmTeam(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	full := c.can("team", 'v') || c.can("reports", 'v')
	mem := []map[string]any{}
	for _, m := range c.members() {
		x := map[string]any{"uid": m["uid"], "name": m["name"], "role": m["role"], "dept": m["dept"], "status": m["status"]}
		if full {
			x["email"], x["scope"] = m["email"], m["scope"]
		}
		mem = append(mem, x)
	}
	ds := []map[string]any{}
	for _, d := range c.depts() {
		ds = append(ds, map[string]any{"id": d.ID, "name": d.Name, "parentId": d.Parent, "headUid": d.Head})
	}
	out := map[string]any{"members": mem, "departments": ds}
	if c.can("team", 'v') {
		inv := []map[string]any{}
		it := s.db.Collection("crm_invites").Where("wid", "==", c.wid).Documents(c.ctx)
		for {
			d, err := it.Next()
			if err != nil {
				break
			}
			x := d.Data()
			x["email"] = d.Ref.ID
			inv = append(inv, x)
		}
		it.Stop()
		out["invites"] = inv
	}
	jsonOut(w, out)
}

func (c *crmReq) targetOK(w http.ResponseWriter, tuid string, newRole string) (map[string]any, bool) {
	td, err := c.s.crmWS(c.wid).Collection("members").Doc(tuid).Get(c.ctx)
	if err != nil {
		http.Error(w, "Member not found", 404)
		return nil, false
	}
	t := td.Data()
	if tuid != c.uid && crmRank[sv(t, "role")] >= crmRank[c.role] {
		http.Error(w, "You can only manage people with a lower role than yours.", 403)
		return nil, false
	}
	if newRole != "" && (newRole == "owner" || crmRank[newRole] == 0 || crmRank[newRole] >= crmRank[c.role]) {
		http.Error(w, "You can only assign roles lower than your own.", 403)
		return nil, false
	}
	return t, true
}

func (s *server) crmMemberSave(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	uid := in["uid"]
	if uid == "" {
		http.Error(w, "bad request", 400)
		return
	}
	self := uid == c.uid
	if !self && !c.can("team", 'e') {
		c.deny(w, "edit team members")
		return
	}
	ups := []firestore.Update{}
	if n := clip(in["name"], 60); n != "" {
		ups = append(ups, firestore.Update{Path: "name", Value: n})
	}
	if !self || c.role == "owner" || c.role == "admin" {
		role := in["role"]
		if self && role != "" && role != c.role {
			http.Error(w, "You cannot change your own role.", 403)
			return
		}
		if _, ok := c.targetOK(w, uid, ifs(self, "", role)); !ok {
			return
		}
		if self && (in["status"] == "disabled") {
			http.Error(w, "You cannot turn off your own access.", 403)
			return
		}
		if role != "" && !self {
			ups = append(ups, firestore.Update{Path: "role", Value: role})
		}
		if v, ok := in["dept"]; ok && c.can("team", 'e') {
			if v != "" {
				if _, err := c.s.crmWS(c.wid).Collection("departments").Doc(v).Get(c.ctx); err != nil {
					http.Error(w, "Unknown department", 400)
					return
				}
			}
			ups = append(ups, firestore.Update{Path: "dept", Value: v})
		}
		if v := in["scope"]; (v == "own" || v == "dept" || v == "all" || v == "default") && !self {
			if v == "default" {
				v = ""
			}
			ups = append(ups, firestore.Update{Path: "scope", Value: v})
		}
		if v := in["status"]; (v == "active" || v == "disabled") && !self {
			ups = append(ups, firestore.Update{Path: "status", Value: v})
		}
	}
	if len(ups) == 0 {
		jsonOut(w, map[string]bool{"ok": true})
		return
	}
	if _, err := c.s.crmWS(c.wid).Collection("members").Doc(uid).Update(c.ctx, ups); err != nil {
		http.Error(w, "db error", 500)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

func (s *server) crmMemberRemove(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	if !c.can("team", 'd') || in["uid"] == c.uid {
		c.deny(w, "remove this member")
		return
	}
	if _, ok := c.targetOK(w, in["uid"], ""); !ok {
		return
	}
	c.s.crmWS(c.wid).Collection("members").Doc(in["uid"]).Delete(c.ctx)
	c.s.db.Collection("users").Doc(in["uid"]).Collection("private").Doc("crm").Delete(c.ctx)
	jsonOut(w, map[string]bool{"ok": true})
}

func (s *server) crmInvite(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	if !c.can("team", 'c') {
		c.deny(w, "invite people")
		return
	}
	email := strings.ToLower(strings.TrimSpace(in["email"]))
	role := in["role"]
	if !reEmailOK.MatchString(email) || len(email) > 160 {
		http.Error(w, "Enter a valid email address.", 400)
		return
	}
	if role == "owner" || crmRank[role] == 0 || crmRank[role] >= crmRank[c.role] {
		http.Error(w, "You can only invite people with a role lower than your own.", 403)
		return
	}
	for _, m := range c.members() {
		if strings.EqualFold(sv(m, "email"), email) {
			http.Error(w, "That person is already in the workspace.", 400)
			return
		}
	}
	if d, err := s.db.Collection("crm_invites").Doc(email).Get(c.ctx); err == nil && sv(d.Data(), "wid") != c.wid {
		http.Error(w, "That email already has a pending invitation to another workspace.", 400)
		return
	}
	dept := in["dept"]
	if dept != "" {
		if _, err := c.s.crmWS(c.wid).Collection("departments").Doc(dept).Get(c.ctx); err != nil {
			http.Error(w, "Unknown department", 400)
			return
		}
	}
	scope := in["scope"]
	if scope != "own" && scope != "dept" && scope != "all" {
		scope = ""
	}
	s.db.Collection("crm_invites").Doc(email).Set(c.ctx, map[string]any{"wid": c.wid, "role": role, "dept": dept, "scope": scope, "invitedBy": c.uid, "createdAt": time.Now()})
	c.mailInvite(w, r, email, role, true)
}

func baseURL(r *http.Request) string {
	b := strings.TrimRight(env("BASE_URL"), "/")
	if b == "" {
		b = "https://" + r.Host
	}
	return b
}

// mailInvite emails the invitation (when mail is configured) and always returns the link so it can be shared by hand.
func (c *crmReq) mailInvite(w http.ResponseWriter, r *http.Request, email, role string, fresh bool) {
	link := baseURL(r) + "/app"
	out := map[string]any{"ok": true, "joinUrl": link, "mailReady": mailConfigured()}
	if mailConfigured() {
		subj, h, t := inviteMail(sv(c.ws, "name"), firstNonEmptyStr(c.s.crmDisplay(c), "A colleague"), label0(role), link)
		ctx, cancel := context.WithTimeout(c.ctx, 20*time.Second)
		defer cancel()
		if err := sendMail(ctx, email, subj, h, t); err != nil {
			out["emailed"], out["emailError"] = false, err.Error()
		} else {
			out["emailed"] = true
			c.s.db.Collection("crm_invites").Doc(email).Set(c.ctx, map[string]any{"emailedAt": time.Now()}, firestoreMerge())
		}
	} else {
		out["emailed"], out["emailError"] = false, "Email sending is not set up on the server yet, so share the link yourself."
	}
	jsonOut(w, out)
}

func label0(r string) string {
	if r == "" {
		return r
	}
	return strings.ToUpper(r[:1]) + strings.ReplaceAll(r[1:], "_", " ")
}

func (s *server) crmDisplay(c *crmReq) string { return firstNonEmptyStr(sv(c.m, "name"), c.email) }

func (s *server) crmInviteResend(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in["email"]))
	d, err := s.db.Collection("crm_invites").Doc(email).Get(c.ctx)
	if !c.can("team", 'c') || err != nil || sv(d.Data(), "wid") != c.wid {
		c.deny(w, "resend this invitation")
		return
	}
	c.mailInvite(w, r, email, sv(d.Data(), "role"), false)
}

func (s *server) crmInviteCancel(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(in["email"]))
	d, err := s.db.Collection("crm_invites").Doc(email).Get(c.ctx)
	if !c.can("team", 'd') || err != nil || sv(d.Data(), "wid") != c.wid {
		c.deny(w, "cancel this invitation")
		return
	}
	d.Ref.Delete(c.ctx)
	jsonOut(w, map[string]bool{"ok": true})
}

func (s *server) crmDeptSave(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	id := in["id"]
	if (id == "" && !c.can("team", 'c')) || (id != "" && !c.can("team", 'e')) {
		c.deny(w, "manage departments")
		return
	}
	name := clip(in["name"], 60)
	if name == "" {
		http.Error(w, "Enter a department name.", 400)
		return
	}
	parent, head := in["parentId"], in["headUid"]
	ds := c.depts()
	byID := map[string]crmDept{}
	for _, d := range ds {
		byID[d.ID] = d
	}
	if parent != "" {
		if _, ok := byID[parent]; !ok {
			http.Error(w, "Unknown parent department", 400)
			return
		}
		for p, n := parent, 0; p != "" && n < 50; p, n = byID[p].Parent, n+1 {
			if p == id && id != "" {
				http.Error(w, "A department cannot be inside itself.", 400)
				return
			}
		}
	}
	if head != "" && !c.validOwner(head) {
		http.Error(w, "Unknown head of department", 400)
		return
	}
	data := map[string]any{"name": name, "parentId": parent, "headUid": head}
	if id == "" {
		data["createdAt"] = time.Now()
		ref := c.s.crmWS(c.wid).Collection("departments").NewDoc()
		ref.Set(c.ctx, data)
		id = ref.ID
	} else {
		if _, ok := byID[id]; !ok {
			http.Error(w, "Not found", 404)
			return
		}
		c.s.crmWS(c.wid).Collection("departments").Doc(id).Set(c.ctx, data, firestore.MergeAll)
	}
	jsonOut(w, map[string]string{"id": id})
}

func (s *server) crmDeptDelete(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in map[string]string
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	if !c.can("team", 'd') {
		c.deny(w, "delete departments")
		return
	}
	for _, d := range c.depts() {
		if d.Parent == in["id"] {
			http.Error(w, "Move or delete its sub-departments first.", 400)
			return
		}
	}
	for _, m := range c.members() {
		if sv(m, "dept") == in["id"] {
			http.Error(w, "Move its members to another department first.", 400)
			return
		}
	}
	c.s.crmWS(c.wid).Collection("departments").Doc(in["id"]).Delete(c.ctx)
	jsonOut(w, map[string]bool{"ok": true})
}

func (s *server) crmSettings(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("team", 'e') {
		c.deny(w, "change workspace settings")
		return
	}
	var in map[string]any
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	ups := []firestore.Update{}
	if n, _ := in["name"].(string); clip(n, 80) != "" {
		ups = append(ups, firestore.Update{Path: "name", Value: clip(n, 80)})
	}
	if cur, _ := in["currency"].(string); len(strings.TrimSpace(cur)) == 3 {
		ups = append(ups, firestore.Update{Path: "currency", Value: strings.ToUpper(strings.TrimSpace(cur))})
	}
	if a, ok := in["autoAssign"].(bool); ok {
		ups = append(ups, firestore.Update{Path: "autoAssign", Value: a})
	}
	if len(ups) > 0 {
		c.s.crmWS(c.wid).Update(c.ctx, ups)
	}
	jsonOut(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------- revenue dashboard

func (s *server) crmDashboard(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("reports", 'v') {
		c.deny(w, "view reports")
		return
	}
	days := 30
	switch r.URL.Query().Get("days") {
	case "7":
		days = 7
	case "90":
		days = 90
	case "365":
		days = 365
	case "0":
		days = 0
	}
	now := time.Now()
	from := time.Time{}
	if days > 0 {
		from = now.AddDate(0, 0, -days)
	}
	leads, deals, tasks := c.list("leads"), c.list("deals"), c.list("tasks")
	names := map[string]string{"": "Unassigned"}
	for _, m := range c.members() {
		names[sv(m, "uid")] = sv(m, "name")
	}
	type agg struct {
		Key                         string
		Leads, Converted, Won, Open int
		WonValue, Pipeline          float64
	}
	src, own := map[string]*agg{}, map[string]*agg{}
	get := func(m map[string]*agg, k string) *agg {
		if m[k] == nil {
			m[k] = &agg{Key: k}
		}
		return m[k]
	}
	tm := func(d map[string]any, k string) time.Time { t, _ := d[k].(time.Time); return t }
	leadsIn, converted := 0, 0
	statusN := map[string]int{}
	months := map[string]map[string]float64{}
	mk := func(t time.Time) map[string]float64 {
		k := t.Format("2006-01")
		if months[k] == nil {
			months[k] = map[string]float64{}
		}
		return months[k]
	}
	for _, l := range leads {
		statusN[sv(l, "status")]++
		cr := tm(l, "createdAt")
		mk(cr)["leads"]++
		if cr.Before(from) {
			continue
		}
		leadsIn++
		get(src, firstNonEmptyStr(sv(l, "source"), "manual")).Leads++
		get(own, sv(l, "ownerUid")).Leads++
		if sv(l, "status") == "converted" {
			converted++
			get(src, firstNonEmptyStr(sv(l, "source"), "manual")).Converted++
		}
	}
	var won, lost, open int
	var wonVal, pipeline, forecast, cycle float64
	stage := map[string]map[string]float64{}
	for _, st := range crmDealStage {
		stage[st] = map[string]float64{}
	}
	for _, d := range deals {
		st, v := sv(d, "stage"), nv(d, "value")
		o, sc := sv(d, "ownerUid"), firstNonEmptyStr(sv(d, "source"), "manual")
		switch st {
		case "won":
			cl := tm(d, "closedAt")
			if cl.IsZero() {
				cl = tm(d, "updatedAt")
			}
			mm := mk(cl)
			mm["won"]++
			mm["revenue"] += v
			if cl.Before(from) {
				continue
			}
			won++
			wonVal += v
			cycle += cl.Sub(tm(d, "createdAt")).Hours() / 24
			stage[st]["count"]++
			stage[st]["value"] += v
			a, b := get(src, sc), get(own, o)
			a.Won++
			a.WonValue += v
			b.Won++
			b.WonValue += v
		case "lost":
			cl := tm(d, "closedAt")
			if cl.IsZero() {
				cl = tm(d, "updatedAt")
			}
			if !cl.Before(from) {
				lost++
				stage[st]["count"]++
				stage[st]["value"] += v
			}
		default:
			open++
			pipeline += v
			forecast += v * crmStageProb[st]
			stage[st]["count"]++
			stage[st]["value"] += v
			a, b := get(src, sc), get(own, o)
			a.Open++
			a.Pipeline += v
			b.Open++
			b.Pipeline += v
		}
	}
	taskOpen, overdue := 0, 0
	today := now.Format("2006-01-02")
	for _, t := range tasks {
		if t["done"] == true {
			continue
		}
		taskOpen++
		if due := sv(t, "due"); due != "" && due < today {
			overdue++
		}
	}
	rows := func(m map[string]*agg, name bool) []map[string]any {
		var out []map[string]any
		for k, a := range m {
			l := k
			if name {
				l = names[k]
				if l == "" {
					l = "Former member"
				}
			}
			conv := 0.0
			if a.Leads > 0 {
				conv = float64(a.Converted) / float64(a.Leads) * 100
			}
			out = append(out, map[string]any{"key": k, "label": l, "leads": a.Leads, "converted": a.Converted, "convRate": conv, "won": a.Won, "wonValue": a.WonValue, "open": a.Open, "pipeline": a.Pipeline})
		}
		sort.Slice(out, func(i, j int) bool {
			a, b := out[i]["wonValue"].(float64), out[j]["wonValue"].(float64)
			if a != b {
				return a > b
			}
			return out[i]["leads"].(int) > out[j]["leads"].(int)
		})
		return out
	}
	var mkeys []string
	for k := range months {
		if k >= now.AddDate(0, -11, 0).Format("2006-01") {
			mkeys = append(mkeys, k)
		}
	}
	sort.Strings(mkeys)
	var monthly []map[string]any
	for _, k := range mkeys {
		monthly = append(monthly, map[string]any{"month": k, "leads": months[k]["leads"], "won": months[k]["won"], "revenue": months[k]["revenue"]})
	}
	winRate, avg, avgCycle, conv := 0.0, 0.0, 0.0, 0.0
	if won+lost > 0 {
		winRate = float64(won) / float64(won+lost) * 100
	}
	if won > 0 {
		avg, avgCycle = wonVal/float64(won), cycle/float64(won)
	}
	if leadsIn > 0 {
		conv = float64(converted) / float64(leadsIn) * 100
	}
	var stages []map[string]any
	for _, st := range crmDealStage {
		stages = append(stages, map[string]any{"stage": st, "count": stage[st]["count"], "value": stage[st]["value"]})
	}
	jsonOut(w, map[string]any{
		"days": days, "currency": firstNonEmptyStr(sv(c.ws, "currency"), "USD"),
		"kpis": map[string]any{"leads": leadsIn, "converted": converted, "leadConv": conv, "won": won, "revenue": wonVal, "lost": lost, "winRate": winRate, "avgDeal": avg, "avgCycleDays": avgCycle,
			"openDeals": open, "pipeline": pipeline, "forecast": forecast, "tasksOpen": taskOpen, "tasksOverdue": overdue},
		"bySource": rows(src, false), "byOwner": rows(own, true), "stages": stages, "monthly": monthly, "leadStatus": statusN,
	})
}
