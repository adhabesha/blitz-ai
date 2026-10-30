package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const googleScopes = "https://www.googleapis.com/auth/webmasters.readonly https://www.googleapis.com/auth/analytics.readonly https://www.googleapis.com/auth/yt-analytics.readonly https://www.googleapis.com/auth/youtube"

type tokEntry struct {
	tok string
	exp time.Time
}

var tokCache sync.Map // uid -> tokEntry

func (s *server) redirectURI() string {
	return strings.TrimRight(os.Getenv("BASE_URL"), "/") + "/auth/google/callback"
}

func (s *server) googleConfigured() bool {
	return os.Getenv("GOOGLE_CLIENT_ID") != "" && os.Getenv("GOOGLE_CLIENT_SECRET") != "" && os.Getenv("BASE_URL") != ""
}

func (s *server) stateSecret() []byte {
	if v := os.Getenv("STATE_SECRET"); v != "" {
		return []byte(v)
	}
	sum := sha256.Sum256([]byte(os.Getenv("FIREBASE_CREDENTIALS")))
	return sum[:]
}

func (s *server) signState(uid string) string {
	payload := base64.RawURLEncoding.EncodeToString([]byte(uid + "|" + strconv.FormatInt(time.Now().Add(15*time.Minute).Unix(), 10)))
	m := hmac.New(sha256.New, s.stateSecret())
	m.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(m.Sum(nil))
}

func (s *server) checkState(st string) (string, bool) {
	parts := strings.SplitN(st, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	m := hmac.New(sha256.New, s.stateSecret())
	m.Write([]byte(parts[0]))
	if !hmac.Equal([]byte(hex.EncodeToString(m.Sum(nil))), []byte(parts[1])) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	f := strings.SplitN(string(raw), "|", 2)
	if len(f) != 2 {
		return "", false
	}
	exp, _ := strconv.ParseInt(f[1], 10, 64)
	if time.Now().Unix() > exp {
		return "", false
	}
	return f[0], true
}

func (s *server) googleStart(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	if !s.googleConfigured() {
		http.Error(w, "Google connection isn't configured yet (add GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET and BASE_URL in Render).", 400)
		return
	}
	q := url.Values{
		"client_id":              {os.Getenv("GOOGLE_CLIENT_ID")},
		"redirect_uri":           {s.redirectURI()},
		"response_type":          {"code"},
		"scope":                  {googleScopes},
		"access_type":            {"offline"},
		"prompt":                 {"consent"},
		"include_granted_scopes": {"true"},
		"state":                  {s.signState(uid)},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": "https://accounts.google.com/o/oauth2/v2/auth?" + q.Encode()})
}

func (s *server) googleCallback(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.checkState(r.URL.Query().Get("state"))
	code := r.URL.Query().Get("code")
	if !ok || code == "" {
		http.Redirect(w, r, "/dashboard?google=error", http.StatusFound)
		return
	}
	form := url.Values{
		"code":          {code},
		"client_id":     {os.Getenv("GOOGLE_CLIENT_ID")},
		"client_secret": {os.Getenv("GOOGLE_CLIENT_SECRET")},
		"redirect_uri":  {s.redirectURI()},
		"grant_type":    {"authorization_code"},
	}
	resp, err := http.PostForm("https://oauth2.googleapis.com/token", form)
	if err != nil {
		http.Redirect(w, r, "/dashboard?google=error", http.StatusFound)
		return
	}
	defer resp.Body.Close()
	var tk struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&tk) != nil || tk.AccessToken == "" {
		http.Redirect(w, r, "/dashboard?google=error", http.StatusFound)
		return
	}
	doc := s.db.Collection("users").Doc(uid).Collection("private").Doc("google")
	if tk.RefreshToken != "" {
		if _, err := doc.Set(r.Context(), map[string]any{"refreshToken": tk.RefreshToken, "updatedAt": time.Now()}); err != nil {
			http.Redirect(w, r, "/dashboard?google=error", http.StatusFound)
			return
		}
	} else if _, err := doc.Get(r.Context()); err != nil {
		http.Redirect(w, r, "/dashboard?google=error", http.StatusFound)
		return
	}
	tokCache.Store(uid, tokEntry{tok: tk.AccessToken, exp: time.Now().Add(time.Duration(tk.ExpiresIn-120) * time.Second)})
	http.Redirect(w, r, "/dashboard?google=ok", http.StatusFound)
}

func (s *server) accessToken(ctx context.Context, uid string) (string, error) {
	if v, ok := tokCache.Load(uid); ok {
		if e := v.(tokEntry); time.Now().Before(e.exp) {
			return e.tok, nil
		}
	}
	doc, err := s.db.Collection("users").Doc(uid).Collection("private").Doc("google").Get(ctx)
	if err != nil {
		return "", userErr("Connect your Google account first")
	}
	rt, _ := doc.Data()["refreshToken"].(string)
	if rt == "" {
		return "", userErr("Connect your Google account first")
	}
	form := url.Values{
		"refresh_token": {rt},
		"client_id":     {os.Getenv("GOOGLE_CLIENT_ID")},
		"client_secret": {os.Getenv("GOOGLE_CLIENT_SECRET")},
		"grant_type":    {"refresh_token"},
	}
	resp, err := http.PostForm("https://oauth2.googleapis.com/token", form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var tk struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&tk) != nil || tk.AccessToken == "" {
		return "", userErr("Your Google connection expired. Click Reconnect Google.")
	}
	tokCache.Store(uid, tokEntry{tok: tk.AccessToken, exp: time.Now().Add(time.Duration(tk.ExpiresIn-120) * time.Second)})
	return tk.AccessToken, nil
}

func gcall(ctx context.Context, tok, method, u string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("google %d: %s", resp.StatusCode, string(b))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (s *server) googleStatus(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	_, derr := s.db.Collection("users").Doc(uid).Collection("private").Doc("google").Get(r.Context())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"configured": s.googleConfigured(), "connected": derr == nil})
}

func (s *server) googleDisconnect(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	doc := s.db.Collection("users").Doc(uid).Collection("private").Doc("google")
	if d, err := doc.Get(r.Context()); err == nil {
		if rt, _ := d.Data()["refreshToken"].(string); rt != "" {
			http.PostForm("https://oauth2.googleapis.com/revoke", url.Values{"token": {rt}})
		}
	}
	doc.Delete(r.Context())
	tokCache.Delete(uid)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func (s *server) googleToken(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	tok, err := s.accessToken(r.Context(), uid)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"accessToken": tok})
}

type gaProp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *server) googleAssets(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	tok, err := s.accessToken(r.Context(), uid)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	out := struct {
		Sites      []string `json:"sites"`
		Properties []gaProp `json:"properties"`
		Channel    string   `json:"channel"`
		Errors     []string `json:"errors"`
	}{Sites: []string{}, Properties: []gaProp{}, Errors: []string{}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	fail := func(m string, err error) {
		mu.Lock()
		out.Errors = append(out.Errors, m+": "+trunc(err.Error(), 160))
		mu.Unlock()
	}
	wg.Add(3)
	go func() {
		defer wg.Done()
		var d struct {
			Entries []struct {
				SiteURL string `json:"siteUrl"`
			} `json:"siteEntry"`
		}
		if err := gcall(ctx, tok, "GET", "https://www.googleapis.com/webmasters/v3/sites", nil, &d); err != nil {
			fail("Search Console", err)
			return
		}
		mu.Lock()
		for _, e := range d.Entries {
			out.Sites = append(out.Sites, e.SiteURL)
		}
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		var d struct {
			Accounts []struct {
				Props []struct {
					Property string `json:"property"`
					Name     string `json:"displayName"`
				} `json:"propertySummaries"`
			} `json:"accountSummaries"`
		}
		if err := gcall(ctx, tok, "GET", "https://analyticsadmin.googleapis.com/v1beta/accountSummaries?pageSize=200", nil, &d); err != nil {
			fail("Google Analytics", err)
			return
		}
		mu.Lock()
		for _, a := range d.Accounts {
			for _, p := range a.Props {
				out.Properties = append(out.Properties, gaProp{ID: strings.TrimPrefix(p.Property, "properties/"), Name: p.Name})
			}
		}
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		var d struct {
			Items []struct {
				Snippet struct {
					Title string `json:"title"`
				} `json:"snippet"`
			} `json:"items"`
		}
		if err := gcall(ctx, tok, "GET", "https://www.googleapis.com/youtube/v3/channels?part=snippet&mine=true", nil, &d); err != nil {
			fail("YouTube", err)
			return
		}
		if len(d.Items) > 0 {
			mu.Lock()
			out.Channel = d.Items[0].Snippet.Title
			mu.Unlock()
		}
	}()
	wg.Wait()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// ---- data fetchers ----

type gscRow struct {
	Keys  []string `json:"keys"`
	Click float64  `json:"clicks"`
	Impr  float64  `json:"impressions"`
	CTR   float64  `json:"ctr"`
	Pos   float64  `json:"position"`
}

type gscData struct {
	Dates                           []string
	Clicks, Impr                    []float64
	CurClicks, CurImpr, CurPos      float64
	PrevClicks, PrevImpr, PrevPos   float64
	Queries                         []gscRow
}

func isoDay(t time.Time) string { return t.Format("2006-01-02") }

func gscQuery(ctx context.Context, tok, site, start, end string, dims []string, limit int) ([]gscRow, error) {
	var d struct {
		Rows []gscRow `json:"rows"`
	}
	u := "https://www.googleapis.com/webmasters/v3/sites/" + url.PathEscape(site) + "/searchAnalytics/query"
	body := map[string]any{"startDate": start, "endDate": end, "dimensions": dims, "rowLimit": limit}
	if err := gcall(ctx, tok, "POST", u, body, &d); err != nil {
		return nil, err
	}
	return d.Rows, nil
}

func sumGSC(rows []gscRow) (clicks, impr, pos float64) {
	var pw float64
	for _, r := range rows {
		clicks += r.Click
		impr += r.Impr
		pw += r.Pos * r.Impr
	}
	if impr > 0 {
		pos = pw / impr
	}
	return
}

func (s *server) fetchGSC(ctx context.Context, tok, site string) (*gscData, error) {
	end := time.Now().UTC().AddDate(0, 0, -3)
	start := end.AddDate(0, 0, -27)
	pEnd := start.AddDate(0, 0, -1)
	pStart := pEnd.AddDate(0, 0, -27)
	cur, err := gscQuery(ctx, tok, site, isoDay(start), isoDay(end), []string{"date"}, 100)
	if err != nil {
		return nil, err
	}
	prev, _ := gscQuery(ctx, tok, site, isoDay(pStart), isoDay(pEnd), []string{"date"}, 100)
	qs, _ := gscQuery(ctx, tok, site, isoDay(start), isoDay(end), []string{"query"}, 250)
	d := &gscData{Queries: qs}
	sort.Slice(cur, func(i, j int) bool { return cur[i].Keys[0] < cur[j].Keys[0] })
	for _, r := range cur {
		d.Dates = append(d.Dates, r.Keys[0])
		d.Clicks = append(d.Clicks, r.Click)
		d.Impr = append(d.Impr, r.Impr)
	}
	d.CurClicks, d.CurImpr, d.CurPos = sumGSC(cur)
	d.PrevClicks, d.PrevImpr, d.PrevPos = sumGSC(prev)
	return d, nil
}

type gaRow struct {
	Dims []string
	Mets []float64
}

func ga4Run(ctx context.Context, tok, prop string, dims, mets []string, start, end string, limit int, order string) ([]gaRow, error) {
	body := map[string]any{"dateRanges": []any{map[string]string{"startDate": start, "endDate": end}}, "limit": limit}
	var dd, mm []any
	for _, x := range dims {
		dd = append(dd, map[string]string{"name": x})
	}
	for _, x := range mets {
		mm = append(mm, map[string]string{"name": x})
	}
	if dd != nil {
		body["dimensions"] = dd
	}
	body["metrics"] = mm
	switch {
	case order == "date":
		body["orderBys"] = []any{map[string]any{"dimension": map[string]string{"dimensionName": "date"}}}
	case strings.HasPrefix(order, "desc:"):
		body["orderBys"] = []any{map[string]any{"metric": map[string]string{"metricName": order[5:]}, "desc": true}}
	}
	var d struct {
		Rows []struct {
			D []struct {
				Value string `json:"value"`
			} `json:"dimensionValues"`
			M []struct {
				Value string `json:"value"`
			} `json:"metricValues"`
		} `json:"rows"`
	}
	if err := gcall(ctx, tok, "POST", "https://analyticsdata.googleapis.com/v1beta/properties/"+prop+":runReport", body, &d); err != nil {
		return nil, err
	}
	var out []gaRow
	for _, r := range d.Rows {
		var row gaRow
		for _, x := range r.D {
			row.Dims = append(row.Dims, x.Value)
		}
		for _, x := range r.M {
			row.Mets = append(row.Mets, atof(x.Value))
		}
		out = append(out, row)
	}
	return out, nil
}

type gaData struct {
	Dates                          []string
	Sessions                       []float64
	CurSess, PrevSess              float64
	CurUsers, PrevUsers            float64
	CurEng, KeyEvents, PrevKey     float64
	HasKey                         bool
	Channels                       []gaRow
	Pages                          []gaRow
}

func (s *server) fetchGA(ctx context.Context, tok, prop string) (*gaData, error) {
	end := time.Now().UTC().AddDate(0, 0, -1)
	start := end.AddDate(0, 0, -27)
	pEnd := start.AddDate(0, 0, -1)
	pStart := pEnd.AddDate(0, 0, -27)
	st, en, ps, pe := isoDay(start), isoDay(end), isoDay(pStart), isoDay(pEnd)
	daily, err := ga4Run(ctx, tok, prop, []string{"date"}, []string{"sessions"}, st, en, 100, "date")
	if err != nil {
		return nil, err
	}
	d := &gaData{}
	for _, r := range daily {
		if len(r.Dims) == 0 || len(r.Mets) == 0 || len(r.Dims[0]) != 8 {
			continue
		}
		x := r.Dims[0]
		d.Dates = append(d.Dates, x[:4]+"-"+x[4:6]+"-"+x[6:])
		d.Sessions = append(d.Sessions, r.Mets[0])
	}
	if t, err := ga4Run(ctx, tok, prop, nil, []string{"sessions", "totalUsers", "engagementRate"}, st, en, 1, ""); err == nil && len(t) > 0 && len(t[0].Mets) >= 3 {
		d.CurSess, d.CurUsers, d.CurEng = t[0].Mets[0], t[0].Mets[1], t[0].Mets[2]
	}
	if t, err := ga4Run(ctx, tok, prop, nil, []string{"sessions", "totalUsers"}, ps, pe, 1, ""); err == nil && len(t) > 0 && len(t[0].Mets) >= 2 {
		d.PrevSess, d.PrevUsers = t[0].Mets[0], t[0].Mets[1]
	}
	if t, err := ga4Run(ctx, tok, prop, nil, []string{"keyEvents"}, st, en, 1, ""); err == nil && len(t) > 0 && len(t[0].Mets) >= 1 {
		d.KeyEvents, d.HasKey = t[0].Mets[0], true
		if p, err := ga4Run(ctx, tok, prop, nil, []string{"keyEvents"}, ps, pe, 1, ""); err == nil && len(p) > 0 && len(p[0].Mets) >= 1 {
			d.PrevKey = p[0].Mets[0]
		}
	}
	d.Channels, _ = ga4Run(ctx, tok, prop, []string{"sessionDefaultChannelGroup"}, []string{"sessions"}, st, en, 8, "desc:sessions")
	d.Pages, _ = ga4Run(ctx, tok, prop, []string{"pagePath"}, []string{"screenPageViews", "engagementRate"}, st, en, 10, "desc:screenPageViews")
	return d, nil
}

type ytaData struct {
	Dates                                  []string
	Views, Gained, Lost                    []float64
	CurViews, PrevViews, CurWatch, PrevWatch float64
	CurGained, CurLost, AvgPct             float64
}

func ytaQuery(ctx context.Context, tok, start, end string) ([][]any, error) {
	q := url.Values{"ids": {"channel==MINE"}, "startDate": {start}, "endDate": {end}, "metrics": {"views,estimatedMinutesWatched,subscribersGained,subscribersLost,averageViewPercentage"}, "dimensions": {"day"}, "sort": {"day"}}
	var d struct {
		Rows [][]any `json:"rows"`
	}
	if err := gcall(ctx, tok, "GET", "https://youtubeanalytics.googleapis.com/v2/reports?"+q.Encode(), nil, &d); err != nil {
		return nil, err
	}
	return d.Rows, nil
}

func fnum(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func (s *server) fetchYTA(ctx context.Context, tok string) (*ytaData, error) {
	end := time.Now().UTC().AddDate(0, 0, -2)
	start := end.AddDate(0, 0, -27)
	pEnd := start.AddDate(0, 0, -1)
	pStart := pEnd.AddDate(0, 0, -27)
	cur, err := ytaQuery(ctx, tok, isoDay(start), isoDay(end))
	if err != nil {
		return nil, err
	}
	prev, _ := ytaQuery(ctx, tok, isoDay(pStart), isoDay(pEnd))
	d := &ytaData{}
	var pctW float64
	for _, r := range cur {
		if len(r) < 6 {
			continue
		}
		day, _ := r[0].(string)
		d.Dates = append(d.Dates, day)
		d.Views = append(d.Views, fnum(r[1]))
		d.Gained = append(d.Gained, fnum(r[3]))
		d.Lost = append(d.Lost, fnum(r[4]))
		d.CurViews += fnum(r[1])
		d.CurWatch += fnum(r[2])
		d.CurGained += fnum(r[3])
		d.CurLost += fnum(r[4])
		pctW += fnum(r[5]) * fnum(r[1])
	}
	if d.CurViews > 0 {
		d.AvgPct = pctW / d.CurViews
	}
	for _, r := range prev {
		if len(r) < 6 {
			continue
		}
		d.PrevViews += fnum(r[1])
		d.PrevWatch += fnum(r[2])
	}
	return d, nil
}

func trendStatus(d float64) string { return rate(d, 0, -10) }

func kpiDelta(label string, cur, prev float64, val string) KPI {
	note := "no previous data"
	st := ""
	if prev > 0 {
		d := delta(cur, prev)
		note = fmt.Sprintf("%+.0f%% vs previous 28 days", d)
		st = trendStatus(d)
	}
	return KPI{Label: label, Value: val, Note: note, Status: st}
}

var reDigits = regexp.MustCompile(`\D`)

func (s *server) evalGoogle(ctx context.Context, uid string, in evalIn) (*Report, error) {
	tok, err := s.accessToken(ctx, uid)
	if err != nil {
		return nil, err
	}
	prop := reDigits.ReplaceAllString(in.Property, "")
	site := strings.TrimSpace(in.Site)
	if site == "" && prop == "" && !in.Channel {
		return nil, userErr("Choose at least one: a Search Console site, a GA4 property, or your YouTube channel")
	}
	var wg sync.WaitGroup
	var gsc *gscData
	var ga *gaData
	var yt *ytaData
	var errs []string
	var mu sync.Mutex
	note := func(src string, err error) {
		mu.Lock()
		errs = append(errs, src+": "+trunc(err.Error(), 140))
		mu.Unlock()
	}
	if site != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := s.fetchGSC(ctx, tok, site)
			if err != nil {
				note("Search Console", err)
				return
			}
			gsc = d
		}()
	}
	if prop != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := s.fetchGA(ctx, tok, prop)
			if err != nil {
				note("Google Analytics", err)
				return
			}
			ga = d
		}()
	}
	if in.Channel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := s.fetchYTA(ctx, tok)
			if err != nil {
				note("YouTube Analytics", err)
				return
			}
			yt = d
		}()
	}
	wg.Wait()
	if gsc == nil && ga == nil && yt == nil {
		return nil, userErr("Could not read data from Google. " + strings.Join(errs, " | ") + " (Check that the APIs are enabled in Google Cloud and that you own the selected items.)")
	}

	r := &Report{Target: "Integrated report", Meta: map[string]string{"site": site}}
	if site != "" && gsc != nil {
		r.Target = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(site, "sc-domain:"), "https://"), "http://")
		r.Target = strings.TrimSuffix(r.Target, "/")
	}
	for _, e := range errs {
		r.Notes = append(r.Notes, "Skipped "+e)
	}
	add := func(c Check) { r.Checks = append(r.Checks, c) }
	const SE, TR, YT = "Search (Google)", "Traffic (GA4)", "YouTube"
	plan := func(p PlanItem) { r.Plan = append(r.Plan, p) }

	if gsc != nil {
		ctr := 0.0
		if gsc.CurImpr > 0 {
			ctr = gsc.CurClicks / gsc.CurImpr * 100
		}
		k := kpiDelta("Search clicks (28d)", gsc.CurClicks, gsc.PrevClicks, human(gsc.CurClicks))
		r.KPIs = append(r.KPIs, k, kpiDelta("Search impressions", gsc.CurImpr, gsc.PrevImpr, human(gsc.CurImpr)),
			KPI{Label: "Average CTR", Value: fmt.Sprintf("%.1f%%", ctr), Note: "Good \u2265 3%", Status: rate(ctr, 3, 1.5)},
			KPI{Label: "Average position", Value: fmt.Sprintf("%.1f", gsc.CurPos), Note: "Lower is better", Status: low(gsc.CurPos, 10, 20)})
		add(chk(SE, "Search clicks trend", ifs(gsc.PrevClicks > 0, trendStatus(delta(gsc.CurClicks, gsc.PrevClicks)), "warn"), fmt.Sprintf("%+.0f%%", delta(gsc.CurClicks, gsc.PrevClicks)), "Clicks are falling. Check for lost rankings, indexing problems or seasonal dips.", 5))
		add(chk(SE, "Click-through rate", rate(ctr, 3, 1.5), fmt.Sprintf("%.1f%%", ctr), "Rewrite page titles and meta descriptions to promise a clear benefit.", 4))
		add(chk(SE, "Average position", low(gsc.CurPos, 10, 20), fmt.Sprintf("%.1f", gsc.CurPos), "Strengthen content and internal links for queries ranking beyond page 1.", 4))
		if ch, total, r2, ok := lineForecast("Search clicks per day + 30-day forecast", gsc.Dates, gsc.Clicks, 30); ok {
			r.Charts = append(r.Charts, ch)
			r.KPIs = append(r.KPIs, KPI{Label: "Forecast clicks (next 30d)", Value: human(total), Note: "trend projection, confidence " + confidence(r2)})
		}
		r.Charts = append(r.Charts, Chart{Title: "Search impressions per day", Type: "line", Labels: shortDates(gsc.Dates), Datasets: []Dataset{{Name: "Impressions", Data: gsc.Impr}}})
		// query tables
		qs := gsc.Queries
		sort.Slice(qs, func(i, j int) bool { return qs[i].Click > qs[j].Click })
		tq := Table{Title: "Top search queries", Columns: []string{"Query", "Clicks", "Impressions", "CTR", "Position"}}
		for i := 0; i < len(qs) && i < 10; i++ {
			tq.Rows = append(tq.Rows, Row{Cells: []string{trunc(qs[i].Keys[0], 50), human(qs[i].Click), human(qs[i].Impr), fmt.Sprintf("%.1f%%", qs[i].CTR*100), fmt.Sprintf("%.1f", qs[i].Pos)}})
		}
		r.Tables = append(r.Tables, tq)
		var strike, lowctr []gscRow
		for _, q := range qs {
			if q.Pos >= 4 && q.Pos <= 20 && q.Impr >= 50 {
				strike = append(strike, q)
			}
			if q.Pos <= 5 && q.Impr >= 100 && q.CTR < 0.03 {
				lowctr = append(lowctr, q)
			}
		}
		extra := func(q gscRow) float64 { return math.Max(0, q.Impr*0.10-q.Click) }
		sort.Slice(strike, func(i, j int) bool { return extra(strike[i]) > extra(strike[j]) })
		ts := Table{Title: "Striking-distance keywords (positions 4-20) \u2013 biggest opportunities", Columns: []string{"Query", "Position", "Impressions", "Clicks", "Est. extra clicks if top 3"}}
		totalExtra := 0.0
		for i, q := range strike {
			totalExtra += extra(q)
			if i < 10 {
				ts.Rows = append(ts.Rows, Row{Cells: []string{trunc(q.Keys[0], 50), fmt.Sprintf("%.1f", q.Pos), human(q.Impr), human(q.Click), "+" + human(extra(q))}})
			}
		}
		if len(strike) > 0 {
			var qn []string
			for i, q := range strike {
				if i < 8 {
					qn = append(qn, clip(q.Keys[0], 60))
				}
			}
			r.Meta["queries"] = strings.Join(qn, ", ")
			r.Tables = append(r.Tables, ts)
			plan(PlanItem{Title: fmt.Sprintf("Improve %d keywords stuck on positions 4-20", len(strike)), Why: fmt.Sprintf("Moving them to the top 3 could add roughly %s clicks per month (estimate using a ~10%% CTR at position 3).", human(totalExtra)), Impact: 5, Effort: 3, Auto: "draft:seo-content"})
		}
		if len(lowctr) > 0 {
			var ln []string
			for i, q := range lowctr {
				if i < 8 {
					ln = append(ln, clip(q.Keys[0], 60))
				}
			}
			r.Meta["lowCtr"] = strings.Join(ln, ", ")
			tl := Table{Title: "High-ranking queries with low CTR \u2013 rewrite title and meta", Columns: []string{"Query", "Position", "Impressions", "CTR"}}
			for i, q := range lowctr {
				if i >= 8 {
					break
				}
				tl.Rows = append(tl.Rows, Row{Cells: []string{trunc(q.Keys[0], 50), fmt.Sprintf("%.1f", q.Pos), human(q.Impr), fmt.Sprintf("%.1f%%", q.CTR*100)}})
			}
			r.Tables = append(r.Tables, tl)
			plan(PlanItem{Title: fmt.Sprintf("Rewrite titles/meta for %d queries that rank well but get few clicks", len(lowctr)), Why: "You already rank in the top 5; a better title and description is the cheapest way to get more clicks.", Impact: 4, Effort: 1, Auto: "draft:meta"})
		}
	}

	if ga != nil {
		r.KPIs = append(r.KPIs, kpiDelta("Sessions (28d)", ga.CurSess, ga.PrevSess, human(ga.CurSess)), kpiDelta("Users", ga.CurUsers, ga.PrevUsers, human(ga.CurUsers)),
			KPI{Label: "Engagement rate", Value: fmt.Sprintf("%.0f%%", ga.CurEng*100), Note: "Good \u2265 60%", Status: rate(ga.CurEng*100, 60, 40)})
		if ga.HasKey {
			r.KPIs = append(r.KPIs, kpiDelta("Key events (conversions)", ga.KeyEvents, ga.PrevKey, human(ga.KeyEvents)))
		}
		add(chk(TR, "Sessions trend", ifs(ga.PrevSess > 0, trendStatus(delta(ga.CurSess, ga.PrevSess)), "warn"), fmt.Sprintf("%+.0f%%", delta(ga.CurSess, ga.PrevSess)), "Traffic is falling. Check campaigns, rankings and tracking.", 5))
		add(chk(TR, "Engagement rate", rate(ga.CurEng*100, 60, 40), fmt.Sprintf("%.0f%%", ga.CurEng*100), "Improve page speed, make the first screen answer the visitor's question, and add clear next steps.", 4))
		if ga.HasKey && ga.CurSess > 0 {
			cr := ga.KeyEvents / ga.CurSess * 100
			add(chk(TR, "Conversion rate (key events per session)", rate(cr, 2, 0.5), fmt.Sprintf("%.1f%%", cr), "Simplify your main call to action and remove steps between landing and conversion.", 5))
		}
		if ch, total, r2, ok := lineForecast("Sessions per day + 30-day forecast", ga.Dates, ga.Sessions, 30); ok {
			r.Charts = append(r.Charts, ch)
			r.KPIs = append(r.KPIs, KPI{Label: "Forecast sessions (next 30d)", Value: human(total), Note: "trend projection, confidence " + confidence(r2)})
		}
		if len(ga.Channels) > 0 {
			cc := Chart{Title: "Sessions by channel (28d)", Type: "bar", Datasets: []Dataset{{Name: "Sessions"}}}
			var tot, organic, top float64
			for _, c := range ga.Channels {
				if len(c.Dims) == 0 || len(c.Mets) == 0 {
					continue
				}
				cc.Labels = append(cc.Labels, c.Dims[0])
				cc.Datasets[0].Data = append(cc.Datasets[0].Data, c.Mets[0])
				tot += c.Mets[0]
				top = math.Max(top, c.Mets[0])
				if c.Dims[0] == "Organic Search" {
					organic = c.Mets[0]
				}
			}
			r.Charts = append(r.Charts, cc)
			if tot > 0 {
				add(chk(TR, "Organic search share", rate(organic/tot*100, 30, 10), fmt.Sprintf("%.0f%% of sessions", organic/tot*100), "Invest in SEO content so more visitors arrive without paid effort.", 3))
				add(chk(TR, "Dependence on one channel", low(top/tot*100, 70, 85), fmt.Sprintf("%.0f%% from the top channel", top/tot*100), "Diversify traffic sources (search, social, email, referrals) to reduce risk.", 2))
				if top/tot > 0.7 {
					plan(PlanItem{Title: "Diversify traffic: one channel brings " + fmt.Sprintf("%.0f%%", top/tot*100) + " of sessions", Why: "Relying on a single source is risky if rankings or ad costs change.", Impact: 3, Effort: 3})
				}
			}
		}
		if len(ga.Pages) > 0 {
			tp := Table{Title: "Top pages (28d)", Columns: []string{"Page", "Views", "Engagement rate"}}
			for _, p := range ga.Pages {
				if len(p.Dims) == 0 || len(p.Mets) < 2 {
					continue
				}
				tp.Rows = append(tp.Rows, Row{Cells: []string{trunc(p.Dims[0], 60), human(p.Mets[0]), fmt.Sprintf("%.0f%%", p.Mets[1]*100)}})
			}
			r.Tables = append(r.Tables, tp)
		}
	}

	if yt != nil {
		net := yt.CurGained - yt.CurLost
		r.KPIs = append(r.KPIs, kpiDelta("YouTube views (28d)", yt.CurViews, yt.PrevViews, human(yt.CurViews)), kpiDelta("Watch hours", yt.CurWatch/60, yt.PrevWatch/60, human(yt.CurWatch/60)),
			KPI{Label: "Net new subscribers", Value: fmt.Sprintf("%+.0f", net), Note: fmt.Sprintf("+%.0f / -%.0f", yt.CurGained, yt.CurLost), Status: ifs(net > 0, "pass", ifs(net == 0, "warn", "fail"))},
			KPI{Label: "Avg % of video watched", Value: fmt.Sprintf("%.0f%%", yt.AvgPct), Note: "Good \u2265 40%", Status: rate(yt.AvgPct, 40, 25)})
		add(chk(YT, "Views trend", ifs(yt.PrevViews > 0, trendStatus(delta(yt.CurViews, yt.PrevViews)), "warn"), fmt.Sprintf("%+.0f%%", delta(yt.CurViews, yt.PrevViews)), "Views are falling. Refresh thumbnails/titles and publish consistently.", 5))
		add(chk(YT, "Audience retention (avg % watched)", rate(yt.AvgPct, 40, 25), fmt.Sprintf("%.0f%%", yt.AvgPct), "Hook viewers in the first 15 seconds and cut slow sections.", 4))
		add(chk(YT, "Net subscriber change", ifs(net > 0, "pass", ifs(net == 0, "warn", "fail")), fmt.Sprintf("%+.0f", net), "Ask for the subscribe at the moment of value and promise what's coming next.", 3))
		if ch, total, r2, ok := lineForecast("YouTube views per day + 30-day forecast", yt.Dates, yt.Views, 30); ok {
			r.Charts = append(r.Charts, ch)
			r.KPIs = append(r.KPIs, KPI{Label: "Forecast views (next 30d)", Value: human(total), Note: "trend projection, confidence " + confidence(r2)})
		}
		r.Charts = append(r.Charts, Chart{Title: "Subscribers gained vs lost per day", Type: "bar", Labels: shortDates(yt.Dates), Datasets: []Dataset{{Name: "Gained", Data: yt.Gained}, {Name: "Lost", Data: yt.Lost}}})
		if yt.AvgPct < 40 {
			plan(PlanItem{Title: "Improve retention: tighter intros and faster pacing", Why: fmt.Sprintf("Viewers watch %.0f%% of your videos on average (good \u2265 40%%).", yt.AvgPct), Impact: 4, Effort: 2})
		}
	}
	if len(r.Plan) == 0 {
		plan(PlanItem{Title: "Keep publishing and re-run this report weekly", Why: "No urgent problems found; consistent tracking reveals what to double down on.", Impact: 2, Effort: 1})
	}
	r.Notes = append(r.Notes, "Google data lags 1-3 days. Forecasts are straight-line trend projections of the last 28 days: useful for direction, not guarantees.")
	r.finish()
	if in.UseAI {
		r.AI = s.insights(ctx, r, "Integrated Google Search Console, Google Analytics and YouTube Analytics report for "+r.Target)
		if r.AI == nil {
			r.Notes = append(r.Notes, "AI written insights were unavailable this time. The measurements above are still accurate.")
		}
	}
	return r, nil
}

func shortDates(d []string) []string {
	out := make([]string, len(d))
	for i, x := range d {
		if len(x) >= 10 {
			out[i] = x[5:10]
		} else {
			out[i] = x
		}
	}
	return out
}
