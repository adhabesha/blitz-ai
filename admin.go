package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

func (s *server) adminOK(r *http.Request) (string, bool) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	t, err := s.auth.VerifyIDToken(r.Context(), tok)
	if err != nil {
		return "", false
	}
	email, _ := t.Claims["email"].(string)
	verified, _ := t.Claims["email_verified"].(bool)
	if !verified || email == "" {
		return email, false
	}
	for _, a := range strings.Split(env("ADMIN_EMAILS"), ",") {
		if strings.EqualFold(strings.TrimSpace(a), email) {
			return email, true
		}
	}
	return email, false
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	email, admin := s.adminOK(r)
	out := map[string]any{"email": email, "admin": admin, "onboarded": false, "goals": []string{}, "role": ""}
	if doc, err := s.db.Collection("users").Doc(uid).Collection("private").Doc("profile").Get(r.Context()); err == nil {
		d := doc.Data()
		if b, ok := d["onboarded"].(bool); ok {
			out["onboarded"] = b
		}
		if rl, ok := d["role"].(string); ok {
			out["role"] = rl
		}
		if a, ok := d["goals"].([]any); ok {
			var g []string
			for _, x := range a {
				if v, ok := x.(string); ok {
					g = append(g, v)
				}
			}
			out["goals"] = g
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *server) saveProfile(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil || r.Method != http.MethodPost {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		Onboarded bool     `json:"onboarded"`
		Goals     []string `json:"goals"`
		Role      string   `json:"role"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	var goals []string
	for _, g := range in.Goals {
		if g = clip(g, 30); g != "" && len(goals) < 8 {
			goals = append(goals, g)
		}
	}
	_, err = s.db.Collection("users").Doc(uid).Collection("private").Doc("profile").Set(r.Context(), map[string]any{"onboarded": in.Onboarded, "goals": goals, "role": clip(in.Role, 30)}, firestore.MergeAll)
	if err != nil {
		http.Error(w, "db error", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func maskVal(v string, secret bool) string {
	if v == "" {
		return ""
	}
	if !secret {
		return v
	}
	if len(v) > 10 {
		return v[:4] + "\u2026" + v[len(v)-3:]
	}
	return "\u2022\u2022\u2022\u2022\u2022\u2022"
}

func (s *server) adminConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOK(r); !ok {
		http.Error(w, "forbidden", 403)
		return
	}
	if r.Method == http.MethodPost {
		var in struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		known := false
		for _, it := range adminItems {
			if it.Key == in.Key {
				known = true
			}
		}
		v := strings.TrimSpace(in.Value)
		if !known || len(v) > 4000 {
			http.Error(w, "unknown setting", 400)
			return
		}
		if in.Key == "BASE_URL" {
			v = strings.TrimRight(v, "/")
		}
		if err := s.setConfig(r.Context(), in.Key, v); err != nil {
			http.Error(w, "could not save", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
		return
	}
	type row struct {
		Key     string `json:"key"`
		Label   string `json:"label"`
		Group   string `json:"group"`
		Help    string `json:"help"`
		Test    string `json:"test"`
		Secret  bool   `json:"secret"`
		Source  string `json:"source"`
		Preview string `json:"preview"`
	}
	var rows []row
	for _, it := range adminItems {
		cfgMu.RLock()
		adm := cfgVals[it.Key]
		cfgMu.RUnlock()
		src, val := "none", ""
		if adm != "" {
			src, val = "admin", adm
		} else if e := env(it.Key); e != "" {
			src, val = "render", e
		}
		rows = append(rows, row{it.Key, it.Label, it.Group, it.Help, it.Test, it.Secret, src, maskVal(val, it.Secret)})
	}
	base := strings.TrimRight(env("BASE_URL"), "/")
	if base == "" {
		base = "https://YOUR-APP.onrender.com"
	}
	redirects := map[string]string{}
	for _, id := range []string{"google", "instagram", "facebook", "tiktok"} {
		redirects[id] = base + "/auth/" + id + "/callback"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"items": rows, "redirects": redirects})
}

func (s *server) adminTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminOK(r); !ok {
		http.Error(w, "forbidden", 403)
		return
	}
	var in struct {
		Target string `json:"target"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	ok, msg := false, ""
	switch in.Target {
	case "gemini":
		if env("GEMINI_API_KEY") == "" {
			msg = "No Gemini key set."
		} else if _, err := s.gemini(ctx, `Reply with the JSON {"ok":true}`); err != nil {
			msg = "Gemini call failed: " + trunc(err.Error(), 120) + " (check the key and model name)"
		} else {
			ok, msg = true, "Gemini works with model "+geminiModel()+"."
		}
	case "youtube":
		var d struct {
			Items []struct{ ID string } `json:"items"`
		}
		if env("YOUTUBE_API_KEY") == "" {
			msg = "No YouTube key set."
		} else if err := ytGet(ctx, "channels", url.Values{"part": {"id"}, "forHandle": {"@YouTube"}}, &d); err != nil {
			msg = trunc(err.Error(), 160)
		} else {
			ok, msg = true, "YouTube Data API key works."
		}
	case "psi":
		if p := s.psi(ctx, "https://example.com"); p.OK {
			ok, msg = true, fmt.Sprintf("PageSpeed works (example.com scored %d).", p.Perf)
		} else {
			msg = "PageSpeed returned nothing. Add or fix PSI_API_KEY (enable the PageSpeed Insights API)."
		}
	case "google":
		ok = s.googleConfigured()
		msg = ifs(ok, "Google sign-in is configured.", "Missing GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET or BASE_URL.")
	case "instagram", "facebook", "tiktok":
		p := provOf(in.Target)
		ok = p != nil && p.configured()
		msg = ifs(ok, "Keys are present. Test by connecting an account in the dashboard.", "Missing app keys or BASE_URL.")
	default:
		http.Error(w, "unknown test", 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": ok, "message": msg})
}
