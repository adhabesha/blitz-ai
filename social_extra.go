package main

// Threads and Reddit (OAuth), Bluesky and Telegram (the person's own key, no developer app needed).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func keyProv(id string) bool { return id == "bluesky" || id == "telegram" }

// keyFields describes the boxes shown for providers that connect with the person's own key.
func keyFields(id string) []map[string]any {
	switch id {
	case "bluesky":
		return []map[string]any{{"k": "a", "label": "Bluesky handle (e.g. name.bsky.social)", "secret": false}, {"k": "b", "label": "App password (Bluesky > Settings > Privacy and security > App passwords)", "secret": true}}
	case "telegram":
		return []map[string]any{{"k": "a", "label": "Bot token (from @BotFather)", "secret": true}, {"k": "b", "label": "Channel or group username (e.g. @mychannel). Add the bot as an admin there.", "secret": false}}
	}
	return nil
}

func extraAuthURL(p *provCfg, state string) string {
	cid, ru := env(p.IDEnv), provRedirect(p.ID)
	switch p.ID {
	case "threads":
		q := url.Values{"client_id": {cid}, "redirect_uri": {ru}, "response_type": {"code"}, "scope": {"threads_basic,threads_manage_insights"}, "state": {state}}
		return "https://threads.net/oauth/authorize?" + q.Encode()
	case "reddit":
		q := url.Values{"client_id": {cid}, "redirect_uri": {ru}, "response_type": {"code"}, "duration": {"permanent"}, "scope": {"identity history read"}, "state": {state}}
		return "https://www.reddit.com/api/v1/authorize?" + q.Encode()
	}
	return ""
}

const redditUA = "web:blitz-ai:1.0 (by /u/blitzai)"

func redditReq(ctx context.Context, method, u string, form url.Values, id, secret, bearer string, out any) error {
	var rd io.Reader
	if form != nil {
		rd = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", redditUA)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if id != "" {
		req.SetBasicAuth(id, secret)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return doJSON(req, out)
}

// extraExchange handles the code exchange for Threads and Reddit. ok is false for other providers.
func extraExchange(ctx context.Context, p *provCfg, code string) (*provToken, bool, error) {
	cid, sec, ru := env(p.IDEnv), env(p.SecretEnv), provRedirect(p.ID)
	now := time.Now().Unix()
	switch p.ID {
	case "threads":
		var a struct {
			AccessToken string `json:"access_token"`
			UserID      any    `json:"user_id"`
		}
		if err := postForm(ctx, "https://graph.threads.net/oauth/access_token", url.Values{"client_id": {cid}, "client_secret": {sec}, "code": {code}, "grant_type": {"authorization_code"}, "redirect_uri": {ru}}, &a); err != nil || a.AccessToken == "" {
			if err == nil {
				err = errors.New("no token returned")
			}
			return nil, true, err
		}
		t := &provToken{Access: a.AccessToken, UserID: fmt.Sprint(a.UserID), Exp: now + 3600}
		var l struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		}
		if getJSON(ctx, "https://graph.threads.net/access_token?"+url.Values{"grant_type": {"th_exchange_token"}, "client_secret": {sec}, "access_token": {a.AccessToken}}.Encode(), &l) == nil && l.AccessToken != "" {
			t.Access, t.Exp = l.AccessToken, now+l.ExpiresIn
		}
		return t, true, nil
	case "reddit":
		var a tokResp
		if err := redditReq(ctx, "POST", "https://www.reddit.com/api/v1/access_token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {ru}}, cid, sec, "", &a); err != nil || a.AccessToken == "" {
			if err == nil {
				err = errors.New("no token returned")
			}
			return nil, true, err
		}
		return &provToken{Access: a.AccessToken, Refresh: a.RefreshToken, Exp: now + ifInt(a.ExpiresIn, 3600)}, true, nil
	}
	return nil, false, nil
}

func ifInt(v, d int64) int64 {
	if v == 0 {
		return d
	}
	return v
}

// extraRefresh renews Threads, Reddit and Bluesky tokens before they run out.
func (s *server) extraRefresh(ctx context.Context, uid string, p *provCfg, t *provToken) {
	now := time.Now().Unix()
	switch p.ID {
	case "threads":
		if t.Exp-now > 7*86400 || t.Exp < now {
			return
		}
		var l struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int64  `json:"expires_in"`
		}
		if getJSON(ctx, "https://graph.threads.net/refresh_access_token?"+url.Values{"grant_type": {"th_refresh_token"}, "access_token": {t.Access}}.Encode(), &l) == nil && l.AccessToken != "" {
			t.Access, t.Exp = l.AccessToken, now+l.ExpiresIn
			s.saveTok(ctx, uid, p.ID, t)
		}
	case "reddit":
		if t.Refresh == "" || t.Exp-now > 600 {
			return
		}
		var a tokResp
		if redditReq(ctx, "POST", "https://www.reddit.com/api/v1/access_token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.Refresh}}, env(p.IDEnv), env(p.SecretEnv), "", &a) == nil && a.AccessToken != "" {
			t.Access, t.Exp = a.AccessToken, now+ifInt(a.ExpiresIn, 3600)
			s.saveTok(ctx, uid, p.ID, t)
		}
	case "bluesky":
		if t.Refresh == "" || t.Exp-now > 900 {
			return
		}
		var a struct {
			Access  string `json:"accessJwt"`
			Refresh string `json:"refreshJwt"`
		}
		req, err := http.NewRequestWithContext(ctx, "POST", "https://bsky.social/xrpc/com.atproto.server.refreshSession", nil)
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "Bearer "+t.Refresh)
		if doJSON(req, &a) == nil && a.Access != "" {
			t.Access, t.Refresh, t.Exp = a.Access, firstNonEmptyStr(a.Refresh, t.Refresh), now+3000
			s.saveTok(ctx, uid, p.ID, t)
		}
	}
}

// ---------------------------------------------------------------- connect with a key

func scrub(err error, secrets ...string) error {
	m := err.Error()
	for _, s := range secrets {
		if s != "" {
			m = strings.ReplaceAll(m, s, "***")
		}
	}
	return errors.New(m)
}

func (s *server) socialConnectKey(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct{ Provider, A, B string }
	if !readBody(w, r, &in, 4<<10) || !keyProv(in.Provider) {
		http.Error(w, "bad request", 400)
		return
	}
	in.A, in.B = strings.TrimSpace(in.A), strings.TrimSpace(in.B)
	if in.A == "" || in.B == "" {
		http.Error(w, "Fill in both boxes.", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	now := time.Now().Unix()
	t := &provToken{}
	switch in.Provider {
	case "bluesky":
		handle := strings.TrimPrefix(strings.ToLower(in.A), "@")
		b, _ := json.Marshal(map[string]string{"identifier": handle, "password": in.B})
		req, _ := http.NewRequestWithContext(ctx, "POST", "https://bsky.social/xrpc/com.atproto.server.createSession", strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		var a struct {
			Access  string `json:"accessJwt"`
			Refresh string `json:"refreshJwt"`
			DID     string `json:"did"`
		}
		if err := doJSON(req, &a); err != nil || a.Access == "" {
			http.Error(w, "Bluesky refused the login. Use your handle and an App password (not your main password).", 400)
			return
		}
		t = &provToken{Access: a.Access, Refresh: a.Refresh, UserID: a.DID, Exp: now + 3000}
	case "telegram":
		ch := in.B
		if !strings.HasPrefix(ch, "@") && !strings.HasPrefix(ch, "-") {
			ch = "@" + ch
		}
		base := "https://api.telegram.org/bot" + in.A
		var me struct {
			OK bool `json:"ok"`
		}
		if err := getJSON(ctx, base+"/getMe", &me); err != nil || !me.OK {
			http.Error(w, "Telegram did not accept that bot token. Copy it again from @BotFather.", 400)
			return
		}
		var cnt struct {
			OK     bool `json:"ok"`
			Result int  `json:"result"`
		}
		if err := getJSON(ctx, base+"/getChatMemberCount?chat_id="+url.QueryEscape(ch), &cnt); err != nil || !cnt.OK {
			http.Error(w, "The bot cannot see that channel. Add the bot as an administrator of "+ch+" and try again.", 400)
			return
		}
		t = &provToken{Access: in.A, UserID: ch}
	}
	if err := s.saveTok(ctx, uid, in.Provider, t); err != nil {
		http.Error(w, "Connected, but saving failed.", 500)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------- data

func fetchThreads(ctx context.Context, tok string) (*socialSnap, error) {
	g := func(path string, q url.Values, out any) error {
		q.Set("access_token", tok)
		return getJSON(ctx, "https://graph.threads.net/v1.0/"+path+"?"+q.Encode(), out)
	}
	var me struct {
		ID, Username string
		Bio          string `json:"threads_biography"`
	}
	if err := g("me", url.Values{"fields": {"id,username,threads_biography"}}, &me); err != nil {
		return nil, err
	}
	snap := &socialSnap{Platform: "threads", Handle: me.Username, Bio: me.Bio}
	var fi struct {
		Data []struct {
			TotalValue struct {
				Value float64 `json:"value"`
			} `json:"total_value"`
		} `json:"data"`
	}
	if g(me.ID+"/threads_insights", url.Values{"metric": {"followers_count"}}, &fi) == nil && len(fi.Data) > 0 {
		snap.Followers = fi.Data[0].TotalValue.Value
	}
	var posts struct {
		Data []struct {
			ID, Text, Permalink, Timestamp string
		} `json:"data"`
	}
	if g("me/threads", url.Values{"fields": {"id,text,permalink,timestamp"}, "limit": {"20"}}, &posts) == nil {
		for i, p := range posts.Data {
			when, _ := time.Parse("2006-01-02T15:04:05-0700", p.Timestamp)
			sp := socialPost{ID: p.ID, Text: p.Text, URL: p.Permalink, When: when}
			if i < 12 {
				var in struct {
					Data []struct {
						Name   string
						Values []struct{ Value float64 }
					} `json:"data"`
				}
				if g(p.ID+"/insights", url.Values{"metric": {"views,likes,replies,reposts,quotes"}}, &in) == nil {
					for _, m := range in.Data {
						if len(m.Values) == 0 {
							continue
						}
						v := m.Values[0].Value
						switch m.Name {
						case "views":
							sp.Views, sp.HasViews = v, v > 0
						case "likes":
							sp.Likes = v
						case "replies":
							sp.Comments = v
						case "reposts", "quotes":
							sp.Shares += v
						}
					}
				}
			}
			snap.Posts = append(snap.Posts, sp)
		}
	}
	return snap, nil
}

func fetchReddit(ctx context.Context, tok string) (*socialSnap, error) {
	var me struct {
		Name      string `json:"name"`
		Karma     float64 `json:"total_karma"`
		Subreddit struct {
			Subscribers float64 `json:"subscribers"`
			Desc        string  `json:"public_description"`
		} `json:"subreddit"`
	}
	if err := redditReq(ctx, "GET", "https://oauth.reddit.com/api/v1/me", nil, "", "", tok, &me); err != nil {
		return nil, err
	}
	snap := &socialSnap{Platform: "reddit", Handle: me.Name, Bio: me.Subreddit.Desc, Followers: me.Subreddit.Subscribers}
	var l struct {
		Data struct {
			Children []struct {
				Data struct {
					ID, Title, Selftext, Permalink string
					Created                        float64 `json:"created_utc"`
					Score, Comments                float64
					NumComments                    float64 `json:"num_comments"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if redditReq(ctx, "GET", "https://oauth.reddit.com/user/"+url.PathEscape(me.Name)+"/submitted?limit=20", nil, "", "", tok, &l) == nil {
		for _, c := range l.Data.Children {
			d := c.Data
			snap.Posts = append(snap.Posts, socialPost{ID: d.ID, Text: firstNonEmptyStr(d.Title, d.Selftext), URL: "https://www.reddit.com" + d.Permalink, When: time.Unix(int64(d.Created), 0), Likes: d.Score, Comments: d.NumComments})
		}
	}
	return snap, nil
}

func fetchBluesky(ctx context.Context, tok, did string) (*socialSnap, error) {
	get := func(path string, out any) error {
		req, err := http.NewRequestWithContext(ctx, "GET", "https://bsky.social/xrpc/"+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		return doJSON(req, out)
	}
	var pr struct {
		Handle      string  `json:"handle"`
		Description string  `json:"description"`
		Followers   float64 `json:"followersCount"`
		Follows     float64 `json:"followsCount"`
	}
	if err := get("app.bsky.actor.getProfile?actor="+url.QueryEscape(did), &pr); err != nil {
		return nil, err
	}
	snap := &socialSnap{Platform: "bluesky", Handle: pr.Handle, Bio: pr.Description, Followers: pr.Followers, Following: pr.Follows}
	var fd struct {
		Feed []struct {
			Post struct {
				URI    string `json:"uri"`
				Record struct {
					Text      string `json:"text"`
					CreatedAt string `json:"createdAt"`
				} `json:"record"`
				Likes, Replies, Reposts, Quotes float64
				LikeCount                       float64 `json:"likeCount"`
				ReplyCount                      float64 `json:"replyCount"`
				RepostCount                     float64 `json:"repostCount"`
				QuoteCount                      float64 `json:"quoteCount"`
			} `json:"post"`
		} `json:"feed"`
	}
	if get("app.bsky.feed.getAuthorFeed?filter=posts_no_replies&limit=20&actor="+url.QueryEscape(did), &fd) == nil {
		for _, f := range fd.Feed {
			p := f.Post
			when, _ := time.Parse(time.RFC3339, p.Record.CreatedAt)
			rk := p.URI[strings.LastIndex(p.URI, "/")+1:]
			snap.Posts = append(snap.Posts, socialPost{ID: p.URI, Text: p.Record.Text, URL: "https://bsky.app/profile/" + pr.Handle + "/post/" + rk, When: when, Likes: p.LikeCount, Comments: p.ReplyCount, Shares: p.RepostCount + p.QuoteCount})
		}
	}
	return snap, nil
}

func fetchTelegram(ctx context.Context, tok, chat string) (*socialSnap, error) {
	base := "https://api.telegram.org/bot" + tok
	var ch struct {
		OK     bool `json:"ok"`
		Result struct {
			Title, Username, Description string
		} `json:"result"`
	}
	if err := getJSON(ctx, base+"/getChat?chat_id="+url.QueryEscape(chat), &ch); err != nil {
		return nil, scrub(err, tok)
	}
	var cnt struct {
		Result float64 `json:"result"`
	}
	if err := getJSON(ctx, base+"/getChatMemberCount?chat_id="+url.QueryEscape(chat), &cnt); err != nil {
		return nil, scrub(err, tok)
	}
	return &socialSnap{Platform: "telegram", Handle: firstNonEmptyStr(ch.Result.Username, ch.Result.Title), Bio: ch.Result.Description, Followers: cnt.Result}, nil
}
