package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type socialPost struct {
	ID, Text, URL                  string
	When                           time.Time
	Likes, Comments, Shares, Views float64
}

type socialSnap struct {
	Platform, Handle, Bio string
	Followers, Following  float64
	Posts                 []socialPost
}

type provCfg struct{ ID, Name, IDEnv, SecretEnv string }

var providers = []provCfg{
	{"instagram", "Instagram", "INSTAGRAM_APP_ID", "INSTAGRAM_APP_SECRET"},
	{"facebook", "Facebook", "FACEBOOK_APP_ID", "FACEBOOK_APP_SECRET"},
	{"tiktok", "TikTok", "TIKTOK_CLIENT_KEY", "TIKTOK_CLIENT_SECRET"},
}

func provOf(id string) *provCfg {
	for i := range providers {
		if providers[i].ID == id {
			return &providers[i]
		}
	}
	return nil
}

func (p *provCfg) configured() bool {
	return env(p.IDEnv) != "" && env(p.SecretEnv) != "" && env("BASE_URL") != ""
}

func provRedirect(id string) string {
	return strings.TrimRight(env("BASE_URL"), "/") + "/auth/" + id + "/callback"
}

func authURL(p *provCfg, state string) string {
	cid, ru := env(p.IDEnv), provRedirect(p.ID)
	switch p.ID {
	case "instagram":
		q := url.Values{"client_id": {cid}, "redirect_uri": {ru}, "response_type": {"code"}, "scope": {"instagram_business_basic,instagram_business_manage_insights"}, "state": {state}}
		return "https://www.instagram.com/oauth/authorize?" + q.Encode()
	case "facebook":
		q := url.Values{"client_id": {cid}, "redirect_uri": {ru}, "response_type": {"code"}, "scope": {"pages_show_list,pages_read_engagement,read_insights"}, "state": {state}}
		return "https://www.facebook.com/v21.0/dialog/oauth?" + q.Encode()
	}
	q := url.Values{"client_key": {cid}, "redirect_uri": {ru}, "response_type": {"code"}, "scope": {"user.info.basic,user.info.profile,user.info.stats,video.list"}, "state": {state}}
	return "https://www.tiktok.com/v2/auth/authorize/?" + q.Encode()
}

func getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return doJSON(req, out)
}

func postForm(ctx context.Context, u string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return doJSON(req, out)
}

func doJSON(req *http.Request, out any) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.ReplaceAll(string(b), "\n", " "))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type provToken struct {
	Access, Refresh, UserID string
	Exp                     int64
}

func exchangeCode(ctx context.Context, p *provCfg, code string) (*provToken, error) {
	cid, sec, ru := env(p.IDEnv), env(p.SecretEnv), provRedirect(p.ID)
	now := time.Now().Unix()
	switch p.ID {
	case "instagram":
		var a struct {
			AccessToken string      `json:"access_token"`
			UserID      json.Number `json:"user_id"`
		}
		form := url.Values{"client_id": {cid}, "client_secret": {sec}, "grant_type": {"authorization_code"}, "redirect_uri": {ru}, "code": {code}}
		if err := postForm(ctx, "https://api.instagram.com/oauth/access_token", form, &a); err != nil {
			return nil, err
		}
		t := &provToken{Access: a.AccessToken, UserID: a.UserID.String(), Exp: now + 3600}
		var l struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		}
		u := "https://graph.instagram.com/access_token?" + url.Values{"grant_type": {"ig_exchange_token"}, "client_secret": {sec}, "access_token": {a.AccessToken}}.Encode()
		if getJSON(ctx, u, &l) == nil && l.AccessToken != "" {
			t.Access, t.Exp = l.AccessToken, now+l.ExpiresIn
		}
		return t, nil
	case "facebook":
		var a struct {
			AccessToken string `json:"access_token"`
		}
		u := "https://graph.facebook.com/v21.0/oauth/access_token?" + url.Values{"client_id": {cid}, "redirect_uri": {ru}, "client_secret": {sec}, "code": {code}}.Encode()
		if err := getJSON(ctx, u, &a); err != nil {
			return nil, err
		}
		t := &provToken{Access: a.AccessToken, Exp: now + 3600}
		var l struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		}
		u2 := "https://graph.facebook.com/v21.0/oauth/access_token?" + url.Values{"grant_type": {"fb_exchange_token"}, "client_id": {cid}, "client_secret": {sec}, "fb_exchange_token": {a.AccessToken}}.Encode()
		if getJSON(ctx, u2, &l) == nil && l.AccessToken != "" {
			t.Access = l.AccessToken
			t.Exp = now + 5184000
			if l.ExpiresIn > 0 {
				t.Exp = now + l.ExpiresIn
			}
		}
		return t, nil
	}
	var a struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	form := url.Values{"client_key": {cid}, "client_secret": {sec}, "code": {code}, "grant_type": {"authorization_code"}, "redirect_uri": {ru}}
	if err := postForm(ctx, "https://open.tiktokapis.com/v2/oauth/token/", form, &a); err != nil || a.AccessToken == "" {
		if err == nil {
			err = fmt.Errorf("no token returned")
		}
		return nil, err
	}
	return &provToken{Access: a.AccessToken, Refresh: a.RefreshToken, Exp: now + a.ExpiresIn}, nil
}

func (s *server) saveTok(ctx context.Context, uid, prov string, t *provToken) error {
	_, err := s.db.Collection("users").Doc(uid).Collection("private").Doc("social_"+prov).Set(ctx, map[string]any{"access": t.Access, "refresh": t.Refresh, "userId": t.UserID, "exp": t.Exp, "updatedAt": time.Now()})
	return err
}

func (s *server) validToken(ctx context.Context, uid string, p *provCfg) (*provToken, error) {
	doc, err := s.db.Collection("users").Doc(uid).Collection("private").Doc("social_" + p.ID).Get(ctx)
	if err != nil {
		return nil, userErr("Connect " + p.Name + " first")
	}
	d := doc.Data()
	t := &provToken{}
	t.Access, _ = d["access"].(string)
	t.Refresh, _ = d["refresh"].(string)
	t.UserID, _ = d["userId"].(string)
	switch v := d["exp"].(type) {
	case int64:
		t.Exp = v
	case float64:
		t.Exp = int64(v)
	}
	if t.Access == "" {
		return nil, userErr("Connect " + p.Name + " first")
	}
	now := time.Now().Unix()
	if p.ID == "instagram" && t.Exp > now && t.Exp-now < 7*86400 {
		var l struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		}
		if getJSON(ctx, "https://graph.instagram.com/refresh_access_token?"+url.Values{"grant_type": {"ig_refresh_token"}, "access_token": {t.Access}}.Encode(), &l) == nil && l.AccessToken != "" {
			t.Access, t.Exp = l.AccessToken, now+l.ExpiresIn
			s.saveTok(ctx, uid, p.ID, t)
		}
	}
	if p.ID == "tiktok" && t.Exp-now < 3600 && t.Refresh != "" {
		var a struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int64  `json:"expires_in"`
		}
		form := url.Values{"client_key": {env(p.IDEnv)}, "client_secret": {env(p.SecretEnv)}, "grant_type": {"refresh_token"}, "refresh_token": {t.Refresh}}
		if postForm(ctx, "https://open.tiktokapis.com/v2/oauth/token/", form, &a) == nil && a.AccessToken != "" {
			t.Access, t.Exp = a.AccessToken, now+a.ExpiresIn
			if a.RefreshToken != "" {
				t.Refresh = a.RefreshToken
			}
			s.saveTok(ctx, uid, p.ID, t)
		}
	}
	if t.Exp > 0 && t.Exp < now {
		return nil, userErr("Your " + p.Name + " connection expired. Click Reconnect.")
	}
	return t, nil
}

// ---- HTTP handlers ----

func (s *server) socialStatus(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	out := map[string]map[string]bool{}
	for i := range providers {
		p := &providers[i]
		_, derr := s.db.Collection("users").Doc(uid).Collection("private").Doc("social_" + p.ID).Get(r.Context())
		out[p.ID] = map[string]bool{"configured": p.configured(), "connected": derr == nil}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"providers": out})
}

func (s *server) socialStart(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		Provider string `json:"provider"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	p := provOf(in.Provider)
	if p == nil || !p.configured() {
		http.Error(w, "This connection isn't set up yet (the owner must add its keys in Render, see SETUP-SOCIAL.md).", 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": authURL(p, s.signState(uid+"~"+p.ID))})
}

func (s *server) socialDisconnect(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		Provider string `json:"provider"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in) != nil || provOf(in.Provider) == nil {
		http.Error(w, "bad request", 400)
		return
	}
	priv := s.db.Collection("users").Doc(uid).Collection("private")
	priv.Doc("social_" + in.Provider).Delete(r.Context())
	priv.Doc("snap_" + in.Provider).Delete(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

func (s *server) socialCallback(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := provOf(id)
		v, ok := s.checkState(r.URL.Query().Get("state"))
		parts := strings.SplitN(v, "~", 2)
		code := strings.TrimSuffix(r.URL.Query().Get("code"), "#_")
		if p == nil || !ok || len(parts) != 2 || parts[1] != id || code == "" {
			http.Redirect(w, r, "/dashboard?social=error&p="+id, http.StatusFound)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		t, err := exchangeCode(ctx, p, code)
		if err != nil {
			log.Println("social exchange:", id, err)
			http.Redirect(w, r, "/dashboard?social=error&p="+id, http.StatusFound)
			return
		}
		if err := s.saveTok(ctx, parts[0], id, t); err != nil {
			log.Println("social save:", err)
			http.Redirect(w, r, "/dashboard?social=error&p="+id, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/dashboard?social=ok&p="+id, http.StatusFound)
	}
}

// ---- data fetchers (official APIs) ----

func fetchInstagram(ctx context.Context, tok string) (*socialSnap, error) {
	base := "https://graph.instagram.com/v21.0/"
	var me struct {
		Username  string  `json:"username"`
		Followers float64 `json:"followers_count"`
		Follows   float64 `json:"follows_count"`
	}
	if err := getJSON(ctx, base+"me?"+url.Values{"fields": {"username,followers_count,follows_count"}, "access_token": {tok}}.Encode(), &me); err != nil {
		return nil, err
	}
	snap := &socialSnap{Platform: "instagram", Handle: me.Username, Followers: me.Followers, Following: me.Follows}
	var bio struct {
		Biography string `json:"biography"`
	}
	if getJSON(ctx, base+"me?"+url.Values{"fields": {"biography"}, "access_token": {tok}}.Encode(), &bio) == nil {
		snap.Bio = bio.Biography
	}
	var md struct {
		Data []struct {
			ID       string  `json:"id"`
			Caption  string  `json:"caption"`
			Time     string  `json:"timestamp"`
			Link     string  `json:"permalink"`
			Likes    float64 `json:"like_count"`
			Comments float64 `json:"comments_count"`
		} `json:"data"`
	}
	if err := getJSON(ctx, base+"me/media?"+url.Values{"fields": {"id,caption,timestamp,permalink,like_count,comments_count"}, "limit": {"30"}, "access_token": {tok}}.Encode(), &md); err != nil {
		return nil, err
	}
	posts := make([]socialPost, len(md.Data))
	var wg sync.WaitGroup
	for i, m := range md.Data {
		t, _ := time.Parse("2006-01-02T15:04:05-0700", m.Time)
		posts[i] = socialPost{ID: m.ID, Text: m.Caption, URL: m.Link, When: t, Likes: m.Likes, Comments: m.Comments}
		if i >= 12 {
			continue
		}
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			for _, met := range []string{"views,reach,saved,shares", "reach,saved,shares", "reach"} {
				var in struct {
					Data []struct {
						Name   string `json:"name"`
						Values []struct {
							Value float64 `json:"value"`
						} `json:"values"`
					} `json:"data"`
				}
				if getJSON(ctx, base+id+"/insights?"+url.Values{"metric": {met}, "access_token": {tok}}.Encode(), &in) != nil {
					continue
				}
				for _, d := range in.Data {
					if len(d.Values) == 0 {
						continue
					}
					v := d.Values[0].Value
					switch d.Name {
					case "views":
						posts[i].Views = v
					case "reach":
						if posts[i].Views == 0 {
							posts[i].Views = v
						}
					case "saved", "shares":
						posts[i].Shares += v
					}
				}
				return
			}
		}(i, m.ID)
	}
	wg.Wait()
	snap.Posts = posts
	return snap, nil
}

func fetchFacebook(ctx context.Context, tok string) (*socialSnap, error) {
	base := "https://graph.facebook.com/v21.0/"
	var acc struct {
		Data []struct {
			ID        string  `json:"id"`
			Name      string  `json:"name"`
			Token     string  `json:"access_token"`
			Followers float64 `json:"followers_count"`
			Fans      float64 `json:"fan_count"`
		} `json:"data"`
	}
	if err := getJSON(ctx, base+"me/accounts?"+url.Values{"fields": {"id,name,access_token,followers_count,fan_count"}, "access_token": {tok}}.Encode(), &acc); err != nil {
		return nil, err
	}
	if len(acc.Data) == 0 {
		return nil, userErr("No Facebook Page found. You must manage a Facebook Page to connect it.")
	}
	pg := acc.Data[0]
	f := pg.Followers
	if f == 0 {
		f = pg.Fans
	}
	snap := &socialSnap{Platform: "facebook", Handle: pg.Name, Followers: f}
	var pd struct {
		Data []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Created string `json:"created_time"`
			Link    string `json:"permalink_url"`
			Shares  struct {
				Count float64 `json:"count"`
			} `json:"shares"`
			Reactions struct {
				Summary struct {
					Total float64 `json:"total_count"`
				} `json:"summary"`
			} `json:"reactions"`
			Comments struct {
				Summary struct {
					Total float64 `json:"total_count"`
				} `json:"summary"`
			} `json:"comments"`
		} `json:"data"`
	}
	fields := "id,message,created_time,permalink_url,shares,reactions.summary(true).limit(0),comments.summary(true).limit(0)"
	if err := getJSON(ctx, base+pg.ID+"/published_posts?"+url.Values{"fields": {fields}, "limit": {"30"}, "access_token": {pg.Token}}.Encode(), &pd); err != nil {
		return nil, err
	}
	for _, p := range pd.Data {
		t, _ := time.Parse("2006-01-02T15:04:05-0700", p.Created)
		snap.Posts = append(snap.Posts, socialPost{ID: p.ID, Text: p.Message, URL: p.Link, When: t, Likes: p.Reactions.Summary.Total, Comments: p.Comments.Summary.Total, Shares: p.Shares.Count})
	}
	return snap, nil
}

func fetchTikTok(ctx context.Context, tok string) (*socialSnap, error) {
	var ui struct {
		Data struct {
			User struct {
				Display   string  `json:"display_name"`
				Username  string  `json:"username"`
				Bio       string  `json:"bio_description"`
				Followers float64 `json:"follower_count"`
				Following float64 `json:"following_count"`
			} `json:"user"`
		} `json:"data"`
	}
	var err error
	for _, f := range []string{"display_name,username,bio_description,follower_count,following_count", "display_name,follower_count,following_count"} {
		if err = gcall(ctx, tok, "GET", "https://open.tiktokapis.com/v2/user/info/?fields="+f, nil, &ui); err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	u := ui.Data.User
	h := u.Username
	if h == "" {
		h = u.Display
	}
	snap := &socialSnap{Platform: "tiktok", Handle: h, Bio: u.Bio, Followers: u.Followers, Following: u.Following}
	var vd struct {
		Data struct {
			Videos []struct {
				ID       string  `json:"id"`
				Title    string  `json:"title"`
				Created  float64 `json:"create_time"`
				Views    float64 `json:"view_count"`
				Likes    float64 `json:"like_count"`
				Comments float64 `json:"comment_count"`
				Shares   float64 `json:"share_count"`
				Link     string  `json:"share_url"`
			} `json:"videos"`
		} `json:"data"`
	}
	if err := gcall(ctx, tok, "POST", "https://open.tiktokapis.com/v2/video/list/?fields=id,title,create_time,view_count,like_count,comment_count,share_count,share_url", map[string]any{"max_count": 20}, &vd); err != nil {
		return nil, err
	}
	for _, v := range vd.Data.Videos {
		snap.Posts = append(snap.Posts, socialPost{ID: v.ID, Text: v.Title, URL: v.Link, When: time.Unix(int64(v.Created), 0), Likes: v.Likes, Comments: v.Comments, Shares: v.Shares, Views: v.Views})
	}
	return snap, nil
}

// followerBaseline stores today's follower count and returns the count from ~30 days ago (0 if unknown).
func (s *server) followerBaseline(ctx context.Context, uid, prov string, now float64) float64 {
	ref := s.db.Collection("users").Doc(uid).Collection("private").Doc("snap_" + prov)
	var days []int64
	var vals []float64
	if doc, err := ref.Get(ctx); err == nil {
		d := doc.Data()
		if a, ok := d["days"].([]any); ok {
			for _, x := range a {
				if v, ok := x.(int64); ok {
					days = append(days, v)
				}
			}
		}
		if a, ok := d["vals"].([]any); ok {
			for _, x := range a {
				switch v := x.(type) {
				case float64:
					vals = append(vals, v)
				case int64:
					vals = append(vals, float64(v))
				}
			}
		}
	}
	if len(days) != len(vals) {
		days, vals = nil, nil
	}
	today := time.Now().Unix() / 86400
	prev, best := 0.0, int64(1<<62)
	for i, d := range days {
		age := today - d
		if age >= 20 && age <= 45 {
			diff := age - 30
			if diff < 0 {
				diff = -diff
			}
			if diff < best {
				best, prev = diff, vals[i]
			}
		}
	}
	if len(days) == 0 || days[len(days)-1] != today {
		days = append(days, today)
		vals = append(vals, now)
		if len(days) > 90 {
			days, vals = days[len(days)-90:], vals[len(vals)-90:]
		}
		ref.Set(ctx, map[string]any{"days": days, "vals": vals})
	}
	return prev
}

func (s *server) evalConnectedSocial(ctx context.Context, uid string, in evalIn) (*Report, error) {
	p := provOf(in.Query)
	if p == nil {
		return nil, userErr("Choose Instagram, Facebook or TikTok")
	}
	t, err := s.validToken(ctx, uid, p)
	if err != nil {
		return nil, err
	}
	var snap *socialSnap
	switch p.ID {
	case "instagram":
		snap, err = fetchInstagram(ctx, t.Access)
	case "facebook":
		snap, err = fetchFacebook(ctx, t.Access)
	default:
		snap, err = fetchTikTok(ctx, t.Access)
	}
	if err != nil {
		if _, ok := err.(userErr); ok {
			return nil, err
		}
		log.Println("fetch", p.ID, err)
		return nil, userErr("Could not read " + p.Name + " data (" + trunc(err.Error(), 180) + "). Try reconnecting; in development mode your own account must be added as a tester.")
	}
	if snap.Followers <= 0 {
		return nil, userErr(p.Name + " returned no follower count. Accounts with very few followers may not expose it yet.")
	}
	posts := snap.Posts
	sort.Slice(posts, func(i, j int) bool { return posts[i].When.After(posts[j].When) })
	now := time.Now()
	var recent []socialPost
	posts30 := 0.0
	for _, q := range posts {
		if now.Sub(q.When).Hours() <= 24*30 {
			posts30++
		}
		if len(recent) < 20 {
			recent = append(recent, q)
		}
	}
	var likes, comments, shares, views float64
	var caps []string
	for i, q := range recent {
		likes += q.Likes
		comments += q.Comments
		shares += q.Shares
		views += q.Views
		if i < 15 && strings.TrimSpace(q.Text) != "" {
			caps = append(caps, strings.TrimSpace(q.Text))
		}
	}
	n := float64(len(recent))
	if n == 0 {
		n = 1
	}
	prev := s.followerBaseline(ctx, uid, p.ID, snap.Followers)
	ein := evalIn{Kind: "social", UseAI: in.UseAI, Platform: p.ID, Handle: snap.Handle, Followers: snap.Followers, PrevFollowers: prev, Following: snap.Following, Posts30: posts30,
		Likes: likes / n, Comments: comments / n, Shares: shares / n, Views: views / n, Bio: snap.Bio, Captions: strings.Join(caps, "\n\n")}
	r, err := s.evalSocial(ctx, ein)
	if err != nil {
		return nil, err
	}
	r.Target = p.Name + " @" + snap.Handle

	// real per-post analytics
	eng := func(q socialPost) float64 { return q.Likes + q.Comments + q.Shares }
	if len(recent) >= 2 {
		ch := Chart{Title: "Engagement per recent post (oldest \u2192 newest)", Type: "bar", Datasets: []Dataset{{Name: "Likes + comments + shares"}}}
		for i := len(recent) - 1; i >= 0; i-- {
			ch.Labels = append(ch.Labels, recent[i].When.Format("01-02"))
			ch.Datasets[0].Data = append(ch.Datasets[0].Data, eng(recent[i]))
		}
		r.Charts = append(r.Charts, ch)
		if views > 0 {
			vc := Chart{Title: "Views / reach per recent post", Type: "line", Datasets: []Dataset{{Name: "Views"}}}
			for i := len(recent) - 1; i >= 0; i-- {
				vc.Labels = append(vc.Labels, recent[i].When.Format("01-02"))
				vc.Datasets[0].Data = append(vc.Datasets[0].Data, recent[i].Views)
			}
			r.Charts = append(r.Charts, vc)
		}
		days := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
		sum, cnt := make([]float64, 7), make([]float64, 7)
		for _, q := range recent {
			sum[q.When.Weekday()] += eng(q)
			cnt[q.When.Weekday()]++
		}
		wc := Chart{Title: "Average engagement by posting weekday", Type: "bar", Labels: days, Datasets: []Dataset{{Name: "Avg engagement"}}}
		bd, ba := -1, 0.0
		for i := 0; i < 7; i++ {
			a := 0.0
			if cnt[i] > 0 {
				a = sum[i] / cnt[i]
			}
			wc.Datasets[0].Data = append(wc.Datasets[0].Data, float64(int(a+0.5)))
			if cnt[i] >= 2 && a > ba {
				bd, ba = i, a
			}
		}
		r.Charts = append(r.Charts, wc)
		byEng := append([]socialPost(nil), recent...)
		sort.Slice(byEng, func(i, j int) bool { return eng(byEng[i]) > eng(byEng[j]) })
		cols := []string{"Post", "Date", "Likes", "Comments", "Shares/saves", "Views"}
		row := func(q socialPost) Row {
			return Row{Cells: []string{trunc(strings.Join(strings.Fields(q.Text), " "), 60), q.When.Format("2006-01-02"), human(q.Likes), human(q.Comments), human(q.Shares), human(q.Views)}}
		}
		top := Table{Title: "Top posts", Columns: cols}
		weak := Table{Title: "Weakest posts \u2013 rewrite these captions", Columns: cols}
		for i := 0; i < len(byEng) && i < 5; i++ {
			top.Rows = append(top.Rows, row(byEng[i]))
			weak.Rows = append(weak.Rows, row(byEng[len(byEng)-1-i]))
		}
		r.Tables = append(r.Tables, top, weak)
		avg := eng(Report0(recent)) / float64(len(recent))
		best := byEng[0]
		ratio := 0.0
		if avg > 0 {
			ratio = eng(best) / avg
		}
		r.Plan = append(r.Plan, PlanItem{Title: "Make 3 more posts like \"" + trunc(strings.Join(strings.Fields(best.Text), " "), 40) + "\"", Why: fmt.Sprintf("Your best recent post earned %.1fx your average engagement. Repeat its topic, hook and format.", ratio), Impact: 5, Effort: 3, Auto: "calendar"})
		if bd >= 0 {
			r.Plan = append(r.Plan, PlanItem{Title: "Post on " + days[bd] + "s more often", Why: fmt.Sprintf("Posts published on %s average %s engagement, your strongest day (small sample).", days[bd], human(ba)), Impact: 2, Effort: 1, Auto: "calendar"})
			r.Meta["bestDay"] = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}[bd]
		}
		w := byEng[len(byEng)-1]
		if strings.TrimSpace(w.Text) != "" {
			r.Plan = append(r.Plan, PlanItem{Title: "Rewrite the caption and hashtags of your weakest recent post", Why: "\"" + trunc(strings.Join(strings.Fields(w.Text), " "), 60) + "\" earned the least engagement. Reuse the idea with a stronger hook.", Impact: 3, Effort: 1})
		}
		r.Meta["topTitle"] = clip(best.Text, 120)
	}
	if len(caps) > 0 {
		r.Meta["niche"] = clip(strings.Join(caps[:minInt(3, len(caps))], " | "), 300)
	}
	r.Notes = append(r.Notes, fmt.Sprintf("Data comes straight from %s's official API for your last %d posts. Follower growth appears after a second check at least 20 days later.", p.Name, len(recent)))
	return r, nil
}

// Report0 sums engagement of posts into a pseudo post so averages share one helper.
func Report0(ps []socialPost) socialPost {
	var t socialPost
	for _, q := range ps {
		t.Likes += q.Likes
		t.Comments += q.Comments
		t.Shares += q.Shares
	}
	return t
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
