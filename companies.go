package main

// Company workspaces.
//
// A user can work in one of three modes (kept in their profile, so web and mobile agree):
//   ficx    – the stand-alone analytics tools only
//   hybrid  – analytics plus the CRM, everything shared (the original layout)
//   company – everything scoped to one company: its social accounts, reports, posts, ads, chats,
//             automations, outbound and CRM records
//
// Nothing has to be set up twice. Accounts connected before any company exists are the "main" accounts, and a
// company can simply reuse them (useMain). A company that needs its own accounts (an agency client, say) gets a
// separate connection namespace: every request that carries an X-Company header is served under the user id
// "<uid>~<companyId>", so connections, reports, schedules and ad settings are kept apart without touching the
// feature code. CRM records are tagged with the company id and filtered when a company is active.

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

var reCID = regexp.MustCompile(`^[a-f0-9]{8,16}$`)

type coInfo struct {
	Name    string
	UseMain bool
	Owner   string // whose accounts the company uses: the person who created it
	Shared  bool   // created by a teammate in the same CRM workspace
}

type coEntry struct {
	info coInfo
	ok   bool
	exp  time.Time
}

var coCache = struct {
	sync.Mutex
	m map[string]coEntry
}{m: map[string]coEntry{}}

func coForget(real, cid string) {
	coCache.Lock()
	delete(coCache.m, real+"/"+cid)
	coCache.Unlock()
}

// Roles that may work inside a shared company (publish, ads, automations...).
var coRoles = map[string]bool{"owner": true, "admin": true, "manager": true, "marketing": true}

func (s *server) userWID(ctx context.Context, uid string) string {
	d, err := s.db.Collection("users").Doc(uid).Collection("private").Doc("crm").Get(ctx)
	if err != nil {
		return ""
	}
	return sv(d.Data(), "wid")
}

// workspaceRole returns the person's active role in their CRM workspace ("" if none).
func (s *server) workspaceRole(ctx context.Context, wid, uid string) string {
	d, err := s.crmWS(wid).Collection("members").Doc(uid).Get(ctx)
	if err != nil || sv(d.Data(), "status") == "disabled" {
		return ""
	}
	return sv(d.Data(), "role")
}

// companyInfo finds a company the person may use: one they created, or one shared through their CRM workspace
// (marketing roles and above). Cached for a minute.
func (s *server) companyInfo(ctx context.Context, real, cid string) (coInfo, bool) {
	if !reCID.MatchString(cid) {
		return coInfo{}, false
	}
	key := real + "/" + cid
	coCache.Lock()
	e, hit := coCache.m[key]
	coCache.Unlock()
	if hit && time.Now().Before(e.exp) {
		return e.info, e.ok
	}
	var info coInfo
	ok := false
	if d, err := s.db.Collection("users").Doc(real).Collection("companies").Doc(cid).Get(ctx); err == nil {
		x := d.Data()
		info = coInfo{Name: sv(x, "name"), Owner: real}
		info.UseMain, _ = x["useMain"].(bool)
		ok = true
	} else if wid := s.userWID(ctx, real); wid != "" {
		if d, err := s.crmWS(wid).Collection("brands").Doc(cid).Get(ctx); err == nil && coRoles[s.workspaceRole(ctx, wid, real)] {
			x := d.Data()
			info = coInfo{Name: sv(x, "name"), Owner: firstNonEmptyStr(sv(x, "owner"), real)}
			info.UseMain, _ = x["useMain"].(bool)
			info.Shared = info.Owner != real
			ok = true
		}
	}
	coCache.Lock()
	coCache.m[key] = coEntry{info, ok, time.Now().Add(time.Minute)}
	coCache.Unlock()
	return info, ok
}

// uid is the data owner for this request: the signed-in person, or the company's namespace when X-Company is set.
// Teammates working in a shared company use the namespace of the person who connected its accounts.
func (s *server) uid(r *http.Request) (string, error) {
	real, err := s.realUID(r)
	if err != nil {
		return "", err
	}
	cid := strings.TrimSpace(r.Header.Get("X-Company"))
	if cid == "" {
		return real, nil
	}
	info, ok := s.companyInfo(r.Context(), real, cid)
	if !ok {
		return "", errors.New("unknown company")
	}
	if info.UseMain {
		return info.Owner, nil
	}
	return info.Owner + "~" + cid, nil
}

var wsModes = map[string]bool{"ficx": true, "hybrid": true, "company": true}

func (s *server) workspaceGet(w http.ResponseWriter, r *http.Request) {
	uid, err := s.realUID(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	mode := ""
	if d, e := s.db.Collection("users").Doc(uid).Collection("private").Doc("ui").Get(r.Context()); e == nil {
		mode = sv(d.Data(), "mode")
	}
	out := []map[string]any{}
	it := s.db.Collection("users").Doc(uid).Collection("companies").Limit(60).Documents(r.Context())
	defer it.Stop()
	for {
		d, e := it.Next()
		if e == iterator.Done || e != nil {
			break
		}
		x := d.Data()
		useMain, _ := x["useMain"].(bool)
		out = append(out, map[string]any{"id": d.Ref.ID, "name": sv(x, "name"), "website": sv(x, "website"), "industry": sv(x, "industry"), "useMain": useMain, "createdAt": x["createdAt"]})
	}
	if wid := s.userWID(r.Context(), uid); wid != "" && coRoles[s.workspaceRole(r.Context(), wid, uid)] {
		it2 := s.crmWS(wid).Collection("brands").Limit(100).Documents(r.Context())
		defer it2.Stop()
		for {
			d, e := it2.Next()
			if e != nil {
				break
			}
			x := d.Data()
			useMain, _ := x["useMain"].(bool)
			out = append(out, map[string]any{"id": d.Ref.ID, "name": sv(x, "name"), "website": sv(x, "website"), "industry": sv(x, "industry"), "useMain": useMain, "createdAt": x["createdAt"], "shared": true})
		}
	}
	sort.Slice(out, func(i, j int) bool { return timeOf(out[i]["createdAt"]).Before(timeOf(out[j]["createdAt"])) })
	jsonOut(w, map[string]any{"mode": mode, "companies": out})
}

func (s *server) workspaceMode(w http.ResponseWriter, r *http.Request) {
	uid, err := s.realUID(r)
	if err != nil || r.Method != http.MethodPost {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct{ Mode string }
	if !readBody(w, r, &in, 1<<10) {
		return
	}
	if !wsModes[in.Mode] {
		http.Error(w, "bad request", 400)
		return
	}
	if _, e := s.db.Collection("users").Doc(uid).Collection("private").Doc("ui").Set(r.Context(), map[string]any{"mode": in.Mode, "updatedAt": time.Now()}, firestoreMerge()); e != nil {
		http.Error(w, "db error", 500)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

// coDoc finds where a company is stored: the person's own list, or the shared list of their CRM workspace
// (owners and administrators, or whoever created it, may change a shared one).
func (s *server) coDoc(ctx context.Context, uid, cid string) (*firestore.DocumentRef, map[string]any, bool) {
	r := s.db.Collection("users").Doc(uid).Collection("companies").Doc(cid)
	if d, e := r.Get(ctx); e == nil {
		return r, d.Data(), true
	}
	if wid := s.userWID(ctx, uid); wid != "" {
		r2 := s.crmWS(wid).Collection("brands").Doc(cid)
		if d, e := r2.Get(ctx); e == nil {
			role := s.workspaceRole(ctx, wid, uid)
			if role == "owner" || role == "admin" || sv(d.Data(), "owner") == uid {
				return r2, d.Data(), true
			}
		}
	}
	return nil, nil, false
}

func (s *server) workspaceCompanySave(w http.ResponseWriter, r *http.Request) {
	uid, err := s.realUID(r)
	if err != nil || r.Method != http.MethodPost {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		ID, Name, Website, Industry string
		UseMain                     bool
	}
	if !readBody(w, r, &in, 4<<10) {
		return
	}
	name := clip(in.Name, 80)
	if name == "" {
		http.Error(w, "Enter the company name.", 400)
		return
	}
	ctx := r.Context()
	site := strings.ToLower(strings.TrimSpace(in.Website))
	site = strings.TrimPrefix(strings.TrimPrefix(site, "https://"), "http://")
	site = strings.TrimSuffix(clip(site, 160), "/")
	data := map[string]any{"name": name, "website": site, "industry": clip(in.Industry, 80), "updatedAt": time.Now()}
	if in.ID != "" {
		if !reCID.MatchString(in.ID) {
			http.Error(w, "bad request", 400)
			return
		}
		ref, _, ok := s.coDoc(ctx, uid, in.ID)
		if !ok {
			http.Error(w, "Company not found, or you cannot change it.", 404)
			return
		}
		if _, e := ref.Set(ctx, data, firestoreMerge()); e != nil {
			http.Error(w, "db error", 500)
			return
		}
		coForgetAll(in.ID)
		jsonOut(w, map[string]string{"id": in.ID})
		return
	}
	// new company: shared with the team when the person leads a CRM workspace, otherwise private to them
	col := s.db.Collection("users").Doc(uid).Collection("companies")
	if wid := s.userWID(ctx, uid); wid != "" {
		switch s.workspaceRole(ctx, wid, uid) {
		case "owner", "admin", "manager":
			col = s.crmWS(wid).Collection("brands")
			data["owner"] = uid
		}
	}
	n := 0
	it := col.Select().Limit(61).Documents(ctx)
	for {
		if _, e := it.Next(); e != nil {
			break
		}
		n++
	}
	it.Stop()
	if n >= 60 {
		http.Error(w, "You reached the limit of 60 companies.", 400)
		return
	}
	id := randHex(6)
	data["useMain"], data["createdAt"] = in.UseMain, time.Now()
	if _, e := col.Doc(id).Set(ctx, data); e != nil {
		http.Error(w, "db error", 500)
		return
	}
	jsonOut(w, map[string]string{"id": id})
}

// coForgetAll drops every cached answer about a company (any teammate's).
func coForgetAll(cid string) {
	coCache.Lock()
	for k := range coCache.m {
		if strings.HasSuffix(k, "/"+cid) {
			delete(coCache.m, k)
		}
	}
	coCache.Unlock()
}

// workspaceCompanyDelete removes the company and the accounts connected only to it. Its CRM records are kept
// (they still show under "All companies") so no sales data is lost by accident.
func (s *server) workspaceCompanyDelete(w http.ResponseWriter, r *http.Request) {
	uid, err := s.realUID(r)
	if err != nil || r.Method != http.MethodPost {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct{ ID string }
	if !readBody(w, r, &in, 1<<10) {
		return
	}
	if !reCID.MatchString(in.ID) {
		http.Error(w, "bad request", 400)
		return
	}
	ctx := r.Context()
	ref, data, ok := s.coDoc(ctx, uid, in.ID)
	if !ok {
		http.Error(w, "Company not found, or you cannot delete it.", 404)
		return
	}
	if useMain, _ := data["useMain"].(bool); !useMain {
		base := firstNonEmptyStr(sv(data, "owner"), uid)
		it := s.db.Collection("users").Doc(base + "~" + in.ID).Collection("private").Documents(ctx)
		for {
			x, e := it.Next()
			if e != nil {
				break
			}
			x.Ref.Delete(ctx)
		}
		it.Stop()
	}
	ref.Delete(ctx)
	coForgetAll(in.ID)
	jsonOut(w, map[string]bool{"ok": true})
}
