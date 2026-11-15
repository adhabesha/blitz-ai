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

	"google.golang.org/api/iterator"
)

var reCID = regexp.MustCompile(`^[a-f0-9]{8,16}$`)

type coInfo struct {
	Name    string
	UseMain bool
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

// companyInfo checks that the company belongs to this user (cached for a minute).
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
	d, err := s.db.Collection("users").Doc(real).Collection("companies").Doc(cid).Get(ctx)
	ok := err == nil
	if ok {
		x := d.Data()
		info.Name = sv(x, "name")
		info.UseMain, _ = x["useMain"].(bool)
	}
	coCache.Lock()
	coCache.m[key] = coEntry{info, ok, time.Now().Add(time.Minute)}
	coCache.Unlock()
	return info, ok
}

// uid is the data owner for this request: the signed-in person, or their company namespace when X-Company is set.
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
		return real, nil
	}
	return real + "~" + cid, nil
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
	site := strings.ToLower(strings.TrimSpace(in.Website))
	site = strings.TrimPrefix(strings.TrimPrefix(site, "https://"), "http://")
	site = strings.TrimSuffix(clip(site, 160), "/")
	col := s.db.Collection("users").Doc(uid).Collection("companies")
	data := map[string]any{"name": name, "website": site, "industry": clip(in.Industry, 80), "updatedAt": time.Now()}
	if in.ID != "" {
		if !reCID.MatchString(in.ID) {
			http.Error(w, "bad request", 400)
			return
		}
		if _, e := col.Doc(in.ID).Get(r.Context()); e != nil {
			http.Error(w, "Company not found.", 404)
			return
		}
		if _, e := col.Doc(in.ID).Set(r.Context(), data, firestoreMerge()); e != nil {
			http.Error(w, "db error", 500)
			return
		}
		coForget(uid, in.ID)
		jsonOut(w, map[string]string{"id": in.ID})
		return
	}
	n := 0
	it := col.Select().Limit(61).Documents(r.Context())
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
	if _, e := col.Doc(id).Set(r.Context(), data); e != nil {
		http.Error(w, "db error", 500)
		return
	}
	jsonOut(w, map[string]string{"id": id})
}

// workspaceCompanyDelete removes the company and the accounts connected only to it. Its CRM records are kept
// (they still show in the "all companies" view) so no sales data is lost by accident.
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
	ref := s.db.Collection("users").Doc(uid).Collection("companies").Doc(in.ID)
	d, e := ref.Get(ctx)
	if e != nil {
		http.Error(w, "Company not found.", 404)
		return
	}
	if useMain, _ := d.Data()["useMain"].(bool); !useMain {
		priv := s.db.Collection("users").Doc(uid + "~" + in.ID).Collection("private")
		it := priv.Documents(ctx)
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
	coForget(uid, in.ID)
	jsonOut(w, map[string]bool{"ok": true})
}
