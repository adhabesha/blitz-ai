package main

import (
	"errors"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// Blitz CRM: workspace, company structure (departments, roles, invites), leads, deals, contacts, companies,
// tasks and activities. Everything lives under crm_workspaces/{wid}. Access is decided on the server from the
// member's role (what they can do in each module) and scope (which records they can see).

const crmMaxRecords = 2000

var crmModules = []string{"leads", "deals", "contacts", "companies", "tasks", "reports", "team", "integrations"}

// v view, c create, e edit, d delete, x import/export
var crmPerms = map[string]map[string]string{
	"owner":     {"leads": "vcedx", "deals": "vcedx", "contacts": "vcedx", "companies": "vcedx", "tasks": "vcedx", "reports": "vx", "team": "vcedx", "integrations": "vced"},
	"admin":     {"leads": "vcedx", "deals": "vcedx", "contacts": "vcedx", "companies": "vcedx", "tasks": "vcedx", "reports": "vx", "team": "vced", "integrations": "vced"},
	"manager":   {"leads": "vcedx", "deals": "vcedx", "contacts": "vcedx", "companies": "vcedx", "tasks": "vcedx", "reports": "vx", "team": "v"},
	"sales":     {"leads": "vce", "deals": "vce", "contacts": "vce", "companies": "vce", "tasks": "vce"},
	"marketing": {"leads": "vcex", "deals": "v", "contacts": "vcex", "companies": "v", "tasks": "vce", "reports": "v", "integrations": "vce"},
	"support":   {"leads": "v", "deals": "v", "contacts": "vce", "companies": "vce", "tasks": "vce"},
	"viewer":    {"leads": "v", "deals": "v", "contacts": "v", "companies": "v", "tasks": "v", "reports": "v"},
}
var crmRank = map[string]int{"owner": 7, "admin": 6, "manager": 5, "marketing": 4, "sales": 3, "support": 2, "viewer": 1}
var crmDefScope = map[string]string{"owner": "all", "admin": "all", "manager": "dept", "marketing": "all", "sales": "own", "support": "own", "viewer": "all"}

var crmLeadStatus = []string{"new", "contacted", "qualified", "unqualified", "converted"}
var crmDealStage = []string{"new", "qualified", "proposal", "negotiation", "won", "lost"}
var crmStageProb = map[string]float64{"new": 0.1, "qualified": 0.25, "proposal": 0.5, "negotiation": 0.75, "won": 1, "lost": 0}

type fspec struct {
	k byte // s string, n number, b bool, a string list, t date text
	m int
}

type entSpec struct {
	module   string
	ownerKey string
	f        map[string]fspec
	enums    map[string][]string
}

var crmEnt = map[string]entSpec{
	"leads": {module: "leads", ownerKey: "ownerUid", f: map[string]fspec{
		"name": {'s', 120}, "email": {'s', 160}, "phone": {'s', 40}, "company": {'s', 120}, "title": {'s', 80}, "source": {'s', 40}, "sourceRef": {'s', 160},
		"status": {'s', 20}, "value": {'n', 0}, "notes": {'s', 3000}, "tags": {'a', 20}, "ownerUid": {'s', 64}, "campaign": {'s', 160}, "adName": {'s', 160}, "formName": {'s', 160}, "country": {'s', 60}, "city": {'s', 80}}, enums: map[string][]string{"status": crmLeadStatus}},
	"deals": {module: "deals", ownerKey: "ownerUid", f: map[string]fspec{
		"title": {'s', 160}, "stage": {'s', 20}, "value": {'n', 0}, "contactId": {'s', 64}, "contactName": {'s', 120}, "companyId": {'s', 64}, "companyName": {'s', 120}, "leadId": {'s', 64},
		"ownerUid": {'s', 64}, "source": {'s', 40}, "expectedClose": {'t', 10}, "lostReason": {'s', 200}, "notes": {'s', 3000}}, enums: map[string][]string{"stage": crmDealStage}},
	"contacts": {module: "contacts", ownerKey: "ownerUid", f: map[string]fspec{
		"name": {'s', 120}, "email": {'s', 160}, "phone": {'s', 40}, "companyId": {'s', 64}, "companyName": {'s', 120}, "title": {'s', 80}, "tags": {'a', 20}, "ownerUid": {'s', 64}, "source": {'s', 40}, "notes": {'s', 3000}, "city": {'s', 80}, "country": {'s', 60}}},
	"companies": {module: "companies", ownerKey: "ownerUid", f: map[string]fspec{
		"name": {'s', 120}, "website": {'s', 160}, "industry": {'s', 80}, "phone": {'s', 40}, "email": {'s', 160}, "address": {'s', 200}, "city": {'s', 80}, "country": {'s', 60}, "size": {'s', 30}, "tags": {'a', 20}, "ownerUid": {'s', 64}, "notes": {'s', 3000}}},
	"tasks": {module: "tasks", ownerKey: "assigneeUid", f: map[string]fspec{
		"title": {'s', 200}, "due": {'t', 10}, "done": {'b', 0}, "priority": {'s', 10}, "assigneeUid": {'s', 64}, "relType": {'s', 12}, "relId": {'s', 64}, "relName": {'s', 120}, "notes": {'s', 2000}}, enums: map[string][]string{"priority": {"low", "normal", "high"}}},
}

// ctext keeps line breaks (clip removes them) for notes and long text.
func ctext(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\r", "")
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func inList(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

func sv(m map[string]any, k string) string { v, _ := m[k].(string); return v }
func nv(m map[string]any, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	case int:
		return float64(v)
	}
	return 0
}

func (s *server) crmWS(wid string) *firestore.DocumentRef {
	return s.db.Collection("crm_workspaces").Doc(wid)
}

// ---------------------------------------------------------------- request context

type crmReq struct {
	s        *server
	r        *http.Request
	ctx      context.Context
	uid      string
	email    string
	verified bool
	wid      string
	m        map[string]any // my member record
	role     string
	scope    string
	dept     string
	ws       map[string]any
	deptSet  map[string]bool // lazily built: uids visible through my department
}

func (s *server) crmAuth(r *http.Request) (*crmReq, error) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	t, err := s.auth.VerifyIDToken(r.Context(), tok)
	if err != nil {
		return nil, err
	}
	if !s.approved(r.Context(), t) {
		return nil, errors.New("awaiting approval")
	}
	c := &crmReq{s: s, r: r, ctx: r.Context(), uid: t.UID}
	c.email, _ = t.Claims["email"].(string)
	c.email = strings.ToLower(strings.TrimSpace(c.email))
	c.verified, _ = t.Claims["email_verified"].(bool)
	if fb, ok := t.Claims["firebase"].(map[string]any); ok {
		if p, _ := fb["sign_in_provider"].(string); p == "google.com" {
			c.verified = true
		}
	}
	return c, nil
}

// load finds the person's workspace. A pending invite for their verified email joins them automatically.
func (c *crmReq) load() (state string, err error) {
	ptr := c.s.db.Collection("users").Doc(c.uid).Collection("private").Doc("crm")
	if d, e := ptr.Get(c.ctx); e == nil {
		c.wid, _ = d.Data()["wid"].(string)
	}
	if c.wid == "" {
		inv := c.s.db.Collection("crm_invites").Doc(c.email)
		if c.email != "" {
			if d, e := inv.Get(c.ctx); e == nil {
				if !c.verified {
					return "unverified", nil
				}
				x := d.Data()
				wid := sv(x, "wid")
				name := c.email
				if i := strings.Index(name, "@"); i > 0 {
					name = name[:i]
				}
				mem := map[string]any{"uid": c.uid, "email": c.email, "name": name, "role": sv(x, "role"), "dept": sv(x, "dept"), "scope": sv(x, "scope"), "status": "active", "joinedAt": time.Now()}
				if _, e := c.s.crmWS(wid).Collection("members").Doc(c.uid).Set(c.ctx, mem); e != nil {
					return "", e
				}
				ptr.Set(c.ctx, map[string]any{"wid": wid})
				inv.Delete(c.ctx)
				c.wid = wid
			}
		}
	}
	if c.wid == "" {
		return "none", nil
	}
	md, e := c.s.crmWS(c.wid).Collection("members").Doc(c.uid).Get(c.ctx)
	if e != nil {
		return "none", nil
	}
	c.m = md.Data()
	if sv(c.m, "status") == "disabled" {
		return "disabled", nil
	}
	c.role = sv(c.m, "role")
	if crmPerms[c.role] == nil {
		c.role = "viewer"
	}
	c.scope = sv(c.m, "scope")
	if c.scope != "own" && c.scope != "dept" && c.scope != "all" {
		c.scope = crmDefScope[c.role]
	}
	if c.role == "owner" || c.role == "admin" {
		c.scope = "all"
	}
	c.dept = sv(c.m, "dept")
	if wd, e := c.s.crmWS(c.wid).Get(c.ctx); e == nil {
		c.ws = wd.Data()
	} else {
		c.ws = map[string]any{}
	}
	return "ok", nil
}

// need authenticates and loads the workspace; it writes the error response itself.
func (s *server) crmNeed(w http.ResponseWriter, r *http.Request) *crmReq {
	c, err := s.crmAuth(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return nil
	}
	st, err := c.load()
	if err != nil {
		http.Error(w, "Could not load your workspace. Try again.", 500)
		return nil
	}
	switch st {
	case "ok":
		return c
	case "unverified":
		http.Error(w, "Verify your email address first: an invitation is waiting for it.", 403)
	case "disabled":
		http.Error(w, "Your access to this workspace was turned off by an administrator.", 403)
	default:
		http.Error(w, "You are not in a CRM workspace yet.", 403)
	}
	return nil
}

func (c *crmReq) can(module string, a byte) bool {
	return strings.IndexByte(crmPerms[c.role][module], a) >= 0
}

func (c *crmReq) deny(w http.ResponseWriter, what string) {
	http.Error(w, "Your role ("+c.role+") is not allowed to "+what+".", 403)
}

func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func readBody(w http.ResponseWriter, r *http.Request, v any, max int64) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, max)).Decode(v); err != nil {
		http.Error(w, "bad request", 400)
		return false
	}
	return true
}

// ---------------------------------------------------------------- scope

type crmDept struct {
	ID, Name, Parent, Head string
}

func (c *crmReq) depts() []crmDept {
	var out []crmDept
	it := c.s.crmWS(c.wid).Collection("departments").Documents(c.ctx)
	defer it.Stop()
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		x := d.Data()
		out = append(out, crmDept{d.Ref.ID, sv(x, "name"), sv(x, "parentId"), sv(x, "headUid")})
	}
	return out
}

func (c *crmReq) members() []map[string]any {
	var out []map[string]any
	it := c.s.crmWS(c.wid).Collection("members").Documents(c.ctx)
	defer it.Stop()
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		x := d.Data()
		x["uid"] = d.Ref.ID
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(sv(out[i], "name")) < strings.ToLower(sv(out[j], "name")) })
	return out
}

// deptUIDs: everyone in my department and the departments below it.
func (c *crmReq) deptUIDs() map[string]bool {
	if c.deptSet != nil {
		return c.deptSet
	}
	set := map[string]bool{c.uid: true}
	if c.dept != "" {
		ds := c.depts()
		in := map[string]bool{c.dept: true}
		for changed := true; changed; {
			changed = false
			for _, d := range ds {
				if !in[d.ID] && in[d.Parent] {
					in[d.ID] = true
					changed = true
				}
			}
		}
		for _, m := range c.members() {
			if in[sv(m, "dept")] {
				set[sv(m, "uid")] = true
			}
		}
	}
	c.deptSet = set
	return set
}

func (c *crmReq) sees(e string, d map[string]any) bool {
	if c.scope == "all" {
		return true
	}
	o := sv(d, crmEnt[e].ownerKey)
	if o == "" && e == "leads" {
		return true // unassigned leads can be claimed
	}
	if o == c.uid || sv(d, "createdBy") == c.uid {
		return true
	}
	return c.scope == "dept" && c.deptUIDs()[o]
}

// ---------------------------------------------------------------- records

func (c *crmReq) list(e string) []map[string]any {
	col := c.s.crmWS(c.wid).Collection(e)
	var it *firestore.DocumentIterator
	if c.scope == "own" && e != "leads" {
		it = col.Where(crmEnt[e].ownerKey, "==", c.uid).Limit(crmMaxRecords).Documents(c.ctx)
	} else {
		it = col.OrderBy("createdAt", firestore.Desc).Limit(crmMaxRecords).Documents(c.ctx)
	}
	defer it.Stop()
	out := []map[string]any{}
	for {
		d, err := it.Next()
		if err == iterator.Done || err != nil {
			break
		}
		x := d.Data()
		x["id"] = d.Ref.ID
		if c.sees(e, x) {
			out = append(out, x)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, _ := out[i]["createdAt"].(time.Time)
		b, _ := out[j]["createdAt"].(time.Time)
		return a.After(b)
	})
	return out
}

func (s *server) crmList(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	e := r.URL.Query().Get("e")
	spec, ok := crmEnt[e]
	if !ok {
		http.Error(w, "bad request", 400)
		return
	}
	if !c.can(spec.module, 'v') {
		c.deny(w, "view "+e)
		return
	}
	jsonOut(w, map[string]any{"items": c.list(e)})
}

// clean validates a client object against the entity spec and returns only the allowed fields.
func (c *crmReq) clean(e string, in map[string]any) (map[string]any, string) {
	spec := crmEnt[e]
	out := map[string]any{}
	for k, fs := range spec.f {
		v, present := in[k]
		if !present {
			continue
		}
		switch fs.k {
		case 's':
			str, _ := v.(string)
			if fs.m > 300 {
				out[k] = ctext(str, fs.m)
			} else {
				out[k] = clip(str, fs.m)
			}
		case 't':
			str, _ := v.(string)
			str = strings.TrimSpace(str)
			if str != "" {
				if _, err := time.Parse("2006-01-02", str); err != nil {
					return nil, "Invalid date in " + k
				}
			}
			out[k] = str
		case 'n':
			f, _ := v.(float64)
			if f < 0 || f > 1e12 {
				return nil, "Invalid number in " + k
			}
			out[k] = f
		case 'b':
			b, _ := v.(bool)
			out[k] = b
		case 'a':
			var l []string
			if arr, ok := v.([]any); ok {
				for _, x := range arr {
					if t, ok := x.(string); ok && strings.TrimSpace(t) != "" && len(l) < fs.m {
						l = append(l, clip(t, 30))
					}
				}
			}
			out[k] = l
		}
		if en := spec.enums[k]; en != nil {
			if str, _ := out[k].(string); !inList(en, str) {
				return nil, "Invalid " + k
			}
		}
	}
	return out, ""
}

func (c *crmReq) validOwner(uid string) bool {
	if uid == "" {
		return true
	}
	d, err := c.s.crmWS(c.wid).Collection("members").Doc(uid).Get(c.ctx)
	return err == nil && sv(d.Data(), "status") != "disabled"
}

func (c *crmReq) displayName(uid string) string {
	if uid == "" {
		return "Unassigned"
	}
	if d, err := c.s.crmWS(c.wid).Collection("members").Doc(uid).Get(c.ctx); err == nil {
		return sv(d.Data(), "name")
	}
	return ""
}

func relName(e string, d map[string]any) string {
	if e == "deals" || e == "tasks" {
		return sv(d, "title")
	}
	return sv(d, "name")
}

func (s *server) crmSave(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	e := r.URL.Query().Get("e")
	spec, ok := crmEnt[e]
	if !ok || r.Method != http.MethodPost {
		http.Error(w, "bad request", 400)
		return
	}
	var in map[string]any
	if !readBody(w, r, &in, 64<<10) {
		return
	}
	id, _ := in["id"].(string)
	data, msg := c.clean(e, in)
	if msg != "" {
		http.Error(w, msg, 400)
		return
	}
	if p, set := data["phone"]; set {
		data["phoneKey"] = normPhone(p.(string))
	}
	title := "name"
	if e == "deals" || e == "tasks" {
		title = "title"
	}
	if id == "" {
		if !c.can(spec.module, 'c') {
			c.deny(w, "create "+e)
			return
		}
		if strings.TrimSpace(sv(data, title)) == "" {
			http.Error(w, "A "+title+" is required.", 400)
			return
		}
		if _, set := data[spec.ownerKey]; !set || sv(data, spec.ownerKey) == "" && c.scope != "all" {
			data[spec.ownerKey] = c.uid
		}
		if !c.validOwner(sv(data, spec.ownerKey)) {
			http.Error(w, "That owner is not a member of this workspace.", 400)
			return
		}
		switch e {
		case "leads":
			if sv(data, "status") == "" {
				data["status"] = "new"
			}
			if sv(data, "source") == "" {
				data["source"] = "manual"
			}
		case "deals":
			if sv(data, "stage") == "" {
				data["stage"] = "new"
			}
		case "tasks":
			if _, ok := data["priority"]; !ok {
				data["priority"] = "normal"
			}
		}
		now := time.Now()
		data["createdAt"], data["updatedAt"], data["createdBy"] = now, now, c.uid
		ref := c.s.crmWS(c.wid).Collection(e).NewDoc()
		if _, err := ref.Set(c.ctx, data); err != nil {
			http.Error(w, "db error", 500)
			return
		}
		if e != "tasks" {
			c.s.crmLog(c.ctx, c.wid, e, ref.ID, relName(e, data), "created", "Created", c.uid, sv(data, spec.ownerKey))
		}
		data["id"] = ref.ID
		jsonOut(w, data)
		return
	}
	// update
	if !c.can(spec.module, 'e') {
		c.deny(w, "edit "+e)
		return
	}
	ref := c.s.crmWS(c.wid).Collection(e).Doc(id)
	old, err := ref.Get(c.ctx)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	od := old.Data()
	if !c.sees(e, od) {
		c.deny(w, "change this record (it belongs to someone outside your scope)")
		return
	}
	if o, set := data[spec.ownerKey]; set && !c.validOwner(o.(string)) {
		http.Error(w, "That owner is not a member of this workspace.", 400)
		return
	}
	if t, set := data[title]; set && strings.TrimSpace(t.(string)) == "" {
		http.Error(w, "A "+title+" is required.", 400)
		return
	}
	if e == "deals" {
		if st, set := data["stage"].(string); set && st != sv(od, "stage") {
			if st == "won" || st == "lost" {
				data["closedAt"] = time.Now()
			} else {
				data["closedAt"] = nil
			}
			if st != "lost" {
				data["lostReason"] = ""
			}
		}
	}
	data["updatedAt"] = time.Now()
	ups := make([]firestore.Update, 0, len(data))
	for k, v := range data {
		ups = append(ups, firestore.Update{Path: k, Value: v})
	}
	if _, err := ref.Update(c.ctx, ups); err != nil {
		http.Error(w, "db error", 500)
		return
	}
	merged := od
	for k, v := range data {
		merged[k] = v
	}
	owner := sv(merged, spec.ownerKey)
	rn := relName(e, merged)
	if e == "deals" && data["stage"] != nil && data["stage"] != od["stage"] {
		txt := "Stage: " + sv(od, "stage") + " → " + sv(data, "stage")
		if sv(data, "stage") == "lost" && sv(data, "lostReason") != "" {
			txt += " (" + sv(data, "lostReason") + ")"
		}
		c.s.crmLog(c.ctx, c.wid, e, id, rn, "stage", txt, c.uid, owner)
	}
	if e == "leads" && data["status"] != nil && data["status"] != od["status"] {
		c.s.crmLog(c.ctx, c.wid, e, id, rn, "stage", "Status: "+sv(od, "status")+" → "+sv(data, "status"), c.uid, owner)
	}
	if o, set := data[spec.ownerKey]; set && o != od[spec.ownerKey] && e != "tasks" {
		c.s.crmLog(c.ctx, c.wid, e, id, rn, "assign", "Assigned to "+c.displayName(owner), c.uid, owner)
	}
	merged["id"] = id
	jsonOut(w, merged)
}

func (s *server) crmDelete(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	e := r.URL.Query().Get("e")
	spec, ok := crmEnt[e]
	var in struct {
		ID string `json:"id"`
	}
	if !ok || r.Method != http.MethodPost || !readBody(w, r, &in, 4<<10) || in.ID == "" {
		http.Error(w, "bad request", 400)
		return
	}
	if !c.can(spec.module, 'd') {
		c.deny(w, "delete "+e)
		return
	}
	ref := c.s.crmWS(c.wid).Collection(e).Doc(in.ID)
	d, err := ref.Get(c.ctx)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	if !c.sees(e, d.Data()) {
		c.deny(w, "delete this record")
		return
	}
	ref.Delete(c.ctx)
	jsonOut(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------- activities

func (s *server) crmLog(ctx context.Context, wid, relType, relID, relName, typ, text, by, owner string) {
	s.crmWS(wid).Collection("activities").NewDoc().Set(ctx, map[string]any{
		"relType": relType, "relId": relID, "relName": clip(relName, 120), "type": typ, "text": ctext(text, 2000), "byUid": by, "ownerUid": owner, "at": time.Now(),
	})
}

func (s *server) crmActivities(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	q := r.URL.Query()
	rt, rid := q.Get("relType"), q.Get("relId")
	col := c.s.crmWS(c.wid).Collection("activities")
	var it *firestore.DocumentIterator
	if rid != "" {
		spec, ok := crmEnt[rt]
		if !ok || !c.can(spec.module, 'v') {
			c.deny(w, "view this history")
			return
		}
		d, err := c.s.crmWS(c.wid).Collection(rt).Doc(rid).Get(c.ctx)
		if err != nil || !c.sees(rt, d.Data()) {
			http.Error(w, "Not found", 404)
			return
		}
		it = col.Where("relId", "==", rid).Limit(200).Documents(c.ctx)
	} else {
		it = col.OrderBy("at", firestore.Desc).Limit(150).Documents(c.ctx)
	}
	defer it.Stop()
	out := []map[string]any{}
	for {
		d, err := it.Next()
		if err != nil {
			break
		}
		x := d.Data()
		x["id"] = d.Ref.ID
		if rid == "" && c.scope != "all" && sv(x, "byUid") != c.uid && sv(x, "ownerUid") != c.uid && !(c.scope == "dept" && c.deptUIDs()[sv(x, "ownerUid")]) {
			continue
		}
		out = append(out, x)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, _ := out[i]["at"].(time.Time)
		b, _ := out[j]["at"].(time.Time)
		return a.After(b)
	})
	if len(out) > 100 {
		out = out[:100]
	}
	jsonOut(w, map[string]any{"items": out})
}

func (s *server) crmAddActivity(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in struct{ RelType, RelID, Type, Text string }
	var raw map[string]string
	if !readBody(w, r, &raw, 8<<10) {
		return
	}
	in.RelType, in.RelID, in.Type, in.Text = raw["relType"], raw["relId"], raw["type"], strings.TrimSpace(raw["text"])
	spec, ok := crmEnt[in.RelType]
	if !ok || in.RelID == "" || in.Text == "" || !inList([]string{"note", "call", "email", "meeting"}, in.Type) {
		http.Error(w, "bad request", 400)
		return
	}
	if !c.can(spec.module, 'e') {
		c.deny(w, "add notes to "+in.RelType)
		return
	}
	d, err := c.s.crmWS(c.wid).Collection(in.RelType).Doc(in.RelID).Get(c.ctx)
	if err != nil || !c.sees(in.RelType, d.Data()) {
		http.Error(w, "Not found", 404)
		return
	}
	c.s.crmLog(c.ctx, c.wid, in.RelType, in.RelID, relName(in.RelType, d.Data()), in.Type, in.Text, c.uid, sv(d.Data(), spec.ownerKey))
	c.s.crmWS(c.wid).Collection(in.RelType).Doc(in.RelID).Update(c.ctx, []firestore.Update{{Path: "updatedAt", Value: time.Now()}})
	jsonOut(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------- convert, import

func (s *server) crmConvert(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	var in struct {
		ID           string  `json:"id"`
		DealTitle    string  `json:"dealTitle"`
		Value        float64 `json:"value"`
		MakeContact  bool    `json:"makeContact"`
		MakeCompany  bool    `json:"makeCompany"`
		ExpectedDate string  `json:"expectedClose"`
	}
	if !readBody(w, r, &in, 8<<10) {
		return
	}
	if in.ID == "" {
		http.Error(w, "bad request", 400)
		return
	}
	if !c.can("leads", 'e') || !c.can("deals", 'c') {
		c.deny(w, "convert leads into deals")
		return
	}
	lref := c.s.crmWS(c.wid).Collection("leads").Doc(in.ID)
	ld, err := lref.Get(c.ctx)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	l := ld.Data()
	if !c.sees("leads", l) {
		c.deny(w, "convert this lead")
		return
	}
	if sv(l, "status") == "converted" {
		http.Error(w, "This lead was already converted.", 400)
		return
	}
	owner := sv(l, "ownerUid")
	if owner == "" {
		owner = c.uid
	}
	now := time.Now()
	res := map[string]any{}
	var companyID, contactID string
	if in.MakeCompany && strings.TrimSpace(sv(l, "company")) != "" && c.can("companies", 'c') {
		ref := c.s.crmWS(c.wid).Collection("companies").NewDoc()
		ref.Set(c.ctx, map[string]any{"name": sv(l, "company"), "ownerUid": owner, "createdAt": now, "updatedAt": now, "createdBy": c.uid, "tags": []string{}, "city": sv(l, "city"), "country": sv(l, "country")})
		companyID = ref.ID
		res["companyId"] = companyID
	}
	if in.MakeContact && c.can("contacts", 'c') {
		ref := c.s.crmWS(c.wid).Collection("contacts").NewDoc()
		ref.Set(c.ctx, map[string]any{"name": sv(l, "name"), "email": sv(l, "email"), "phone": sv(l, "phone"), "title": sv(l, "title"), "companyId": companyID, "companyName": sv(l, "company"), "source": sv(l, "source"), "ownerUid": owner, "createdAt": now, "updatedAt": now, "createdBy": c.uid, "tags": []string{}, "city": sv(l, "city"), "country": sv(l, "country")})
		contactID = ref.ID
		res["contactId"] = contactID
	}
	title := clip(in.DealTitle, 160)
	if title == "" {
		title = firstNonEmptyStr(sv(l, "name"), sv(l, "company"), "New deal")
	}
	value := in.Value
	if value <= 0 {
		value = nv(l, "value")
	}
	if in.ExpectedDate != "" {
		if _, err := time.Parse("2006-01-02", in.ExpectedDate); err != nil {
			in.ExpectedDate = ""
		}
	}
	var brief []string
	if n := strings.TrimSpace(sv(l, "notes")); n != "" {
		brief = append(brief, n)
	}
	for _, k := range [][2]string{{"campaign", "Campaign"}, {"source", "Source"}, {"email", "Email"}, {"phone", "Phone"}} {
		if v := sv(l, k[0]); v != "" {
			brief = append(brief, k[1]+": "+v)
		}
	}
	dref := c.s.crmWS(c.wid).Collection("deals").NewDoc()
	dref.Set(c.ctx, map[string]any{"notes": clip(strings.Join(brief, "\n"), 2000), "title": title, "stage": "qualified", "value": value, "contactId": contactID, "contactName": sv(l, "name"), "companyId": companyID, "companyName": sv(l, "company"), "leadId": in.ID,
		"ownerUid": owner, "source": sv(l, "source"), "expectedClose": in.ExpectedDate, "createdAt": now, "updatedAt": now, "createdBy": c.uid})
	res["dealId"] = dref.ID
	lref.Update(c.ctx, []firestore.Update{{Path: "status", Value: "converted"}, {Path: "updatedAt", Value: now}, {Path: "dealId", Value: dref.ID}})
	c.s.crmLog(c.ctx, c.wid, "leads", in.ID, sv(l, "name"), "converted", "Converted into deal \""+title+"\"", c.uid, owner)
	c.s.crmLog(c.ctx, c.wid, "deals", dref.ID, title, "created", "Created from lead "+sv(l, "name"), c.uid, owner)
	jsonOut(w, res)
}

func firstNonEmptyStr(v ...string) string {
	for _, x := range v {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}

func normPhone(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() < 6 {
		return ""
	}
	return b.String()
}

// crmAddLead inserts a lead unless the same email, phone or source reference already exists in the workspace.
// It returns the lead id and whether it was a duplicate (in which case a note is added to the existing lead).
func (s *server) crmAddLead(ctx context.Context, wid string, f map[string]any, by, note string) (string, bool, error) {
	col := s.crmWS(wid).Collection("leads")
	for _, k := range []string{"email", "sourceRef"} {
		if v := strings.ToLower(sv(f, k)); v != "" {
			f[k] = v
			it := col.Where(k, "==", v).Limit(1).Documents(ctx)
			d, err := it.Next()
			it.Stop()
			if err == nil {
				if note != "" {
					s.crmLog(ctx, wid, "leads", d.Ref.ID, sv(d.Data(), "name"), "note", "Contacted us again: "+note, by, sv(d.Data(), "ownerUid"))
				}
				return d.Ref.ID, true, nil
			}
		}
	}
	if p := normPhone(sv(f, "phone")); p != "" {
		it := col.Where("phoneKey", "==", p).Limit(1).Documents(ctx)
		d, err := it.Next()
		it.Stop()
		if err == nil {
			return d.Ref.ID, true, nil
		}
		f["phoneKey"] = p
	}
	now := time.Now()
	f["createdAt"], f["updatedAt"], f["createdBy"] = now, now, by
	if sv(f, "status") == "" {
		f["status"] = "new"
	}
	if f["tags"] == nil {
		f["tags"] = []string{}
	}
	ref := col.NewDoc()
	if _, err := ref.Set(ctx, f); err != nil {
		return "", false, err
	}
	txt := firstNonEmptyStr(note, "Lead added")
	s.crmLog(ctx, wid, "leads", ref.ID, sv(f, "name"), "created", txt+" (source: "+sv(f, "source")+")", by, sv(f, "ownerUid"))
	return ref.ID, false, nil
}

func (s *server) crmImport(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'x') || !c.can("leads", 'c') {
		c.deny(w, "import leads")
		return
	}
	var in struct {
		Rows    []map[string]any `json:"rows"`
		Source  string           `json:"source"`
		OwnerID string           `json:"ownerUid"`
	}
	if !readBody(w, r, &in, 2<<20) {
		return
	}
	if len(in.Rows) == 0 || len(in.Rows) > 1000 {
		http.Error(w, "Import between 1 and 1000 rows at a time.", 400)
		return
	}
	if !c.validOwner(in.OwnerID) {
		http.Error(w, "That owner is not a member of this workspace.", 400)
		return
	}
	owner := in.OwnerID
	if owner == "" && c.scope != "all" {
		owner = c.uid
	}
	src := clip(in.Source, 40)
	if src == "" {
		src = "csv_import"
	}
	created, dup, bad := 0, 0, 0
	for _, row := range in.Rows {
		f, msg := c.clean("leads", row)
		if msg != "" || (sv(f, "name") == "" && sv(f, "email") == "" && sv(f, "phone") == "") {
			bad++
			continue
		}
		if sv(f, "name") == "" {
			f["name"] = firstNonEmptyStr(sv(f, "email"), sv(f, "phone"))
		}
		f["source"], f["ownerUid"] = src, owner
		delete(f, "status")
		_, isDup, err := s.crmAddLead(c.ctx, c.wid, f, c.uid, "Imported from a file")
		switch {
		case err != nil:
			bad++
		case isDup:
			dup++
		default:
			created++
		}
	}
	jsonOut(w, map[string]int{"created": created, "duplicates": dup, "invalid": bad})
}

func firestoreMerge() firestore.SetOption { return firestore.MergeAll }
