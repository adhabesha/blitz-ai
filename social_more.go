package main

// X (Twitter), LinkedIn and Pinterest connections. Each works once its developer keys are added in Admin.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type uidKey struct{}

func pkcePair(uid string) (verifier, challenge string) {
	h := sha256.Sum256([]byte("pkce|" + uid + "|" + env("X_CLIENT_SECRET") + "|" + env("BASE_URL")))
	verifier = base64.RawURLEncoding.EncodeToString(h[:])
	c := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(c[:])
}

func moreAuthURL(p *provCfg, state, uid string) string {
	if u := extraAuthURL(p, state); u != "" {
		return u
	}
	cid, ru := env(p.IDEnv), provRedirect(p.ID)
	switch p.ID {
	case "x":
		_, ch := pkcePair(uid)
		q := url.Values{"response_type": {"code"}, "client_id": {cid}, "redirect_uri": {ru}, "scope": {"tweet.read users.read offline.access"}, "state": {state}, "code_challenge": {ch}, "code_challenge_method": {"S256"}}
		return "https://twitter.com/i/oauth2/authorize?" + q.Encode()
	case "linkedin":
		sc := firstNonEmptyStr(env("LINKEDIN_SCOPES"), "openid profile email"+ifs(env("LINKEDIN_PARTNER") == "1", " r_organization_social rw_organization_admin", ""))
		q := url.Values{"response_type": {"code"}, "client_id": {cid}, "redirect_uri": {ru}, "state": {state}, "scope": {sc}}
		return "https://www.linkedin.com/oauth/v2/authorization?" + q.Encode()
	case "pinterest":
		q := url.Values{"response_type": {"code"}, "client_id": {cid}, "redirect_uri": {ru}, "state": {state}, "scope": {"user_accounts:read,pins:read,boards:read"}}
		return "https://www.pinterest.com/oauth/?" + q.Encode()
	}
	return ""
}

func postFormBasic(ctx context.Context, u string, form url.Values, id, secret string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(id, secret)
	return doJSON(req, out)
}

type tokResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func moreExchange(ctx context.Context, p *provCfg, code string) (*provToken, error) {
	cid, sec, ru := env(p.IDEnv), env(p.SecretEnv), provRedirect(p.ID)
	now := time.Now().Unix()
	var a tokResp
	var err error
	switch p.ID {
	case "x":
		uid, _ := ctx.Value(uidKey{}).(string)
		ver, _ := pkcePair(uid)
		err = postFormBasic(ctx, "https://api.twitter.com/2/oauth2/token", url.Values{"code": {code}, "grant_type": {"authorization_code"}, "client_id": {cid}, "redirect_uri": {ru}, "code_verifier": {ver}}, cid, sec, &a)
	case "linkedin":
		err = postForm(ctx, "https://www.linkedin.com/oauth/v2/accessToken", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {cid}, "client_secret": {sec}, "redirect_uri": {ru}}, &a)
	case "pinterest":
		err = postFormBasic(ctx, "https://api.pinterest.com/v5/oauth/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {ru}}, cid, sec, &a)
	default:
		return nil, fmt.Errorf("unknown provider")
	}
	if err != nil {
		return nil, err
	}
	if a.AccessToken == "" {
		return nil, fmt.Errorf("no token returned")
	}
	if a.ExpiresIn == 0 {
		a.ExpiresIn = 3600
	}
	return &provToken{Access: a.AccessToken, Refresh: a.RefreshToken, Exp: now + a.ExpiresIn}, nil
}

// moreRefresh renews X and Pinterest tokens shortly before they expire.
func (s *server) moreRefresh(ctx context.Context, uid string, p *provCfg, t *provToken) {
	now := time.Now().Unix()
	if t.Refresh == "" || t.Exp-now > 900 {
		return
	}
	var a tokResp
	cid, sec := env(p.IDEnv), env(p.SecretEnv)
	var err error
	switch p.ID {
	case "x":
		err = postFormBasic(ctx, "https://api.twitter.com/2/oauth2/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.Refresh}, "client_id": {cid}}, cid, sec, &a)
	case "pinterest":
		err = postFormBasic(ctx, "https://api.pinterest.com/v5/oauth/token", url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.Refresh}}, cid, sec, &a)
	default:
		return
	}
	if err == nil && a.AccessToken != "" {
		t.Access, t.Exp = a.AccessToken, now+a.ExpiresIn
		if a.RefreshToken != "" {
			t.Refresh = a.RefreshToken
		}
		s.saveTok(ctx, uid, p.ID, t)
	}
}

func fetchX(ctx context.Context, tok string) (*socialSnap, error) {
	var me struct {
		Data struct {
			ID, Username, Description string
			PublicMetrics             struct {
				Followers float64 `json:"followers_count"`
				Following float64 `json:"following_count"`
			} `json:"public_metrics"`
		} `json:"data"`
	}
	if err := gcall(ctx, tok, "GET", "https://api.twitter.com/2/users/me?user.fields=public_metrics,description,username", nil, &me); err != nil {
		if m := err.Error(); strings.Contains(m, "402") || strings.Contains(m, "403") {
			return nil, userErr("X refused the data request. Reading profile and post data needs an X API plan that includes read access (a paid plan). Your connection itself is fine.")
		}
		return nil, err
	}
	snap := &socialSnap{Platform: "x", Handle: me.Data.Username, Bio: me.Data.Description, Followers: me.Data.PublicMetrics.Followers, Following: me.Data.PublicMetrics.Following}
	var tw struct {
		Data []struct {
			ID, Text  string
			CreatedAt string `json:"created_at"`
			M         struct {
				Likes, Replies, Retweets, Quotes, Impressions float64
			} `json:"-"`
			PM struct {
				Like    float64 `json:"like_count"`
				Reply   float64 `json:"reply_count"`
				Retweet float64 `json:"retweet_count"`
				Quote   float64 `json:"quote_count"`
				Impr    float64 `json:"impression_count"`
			} `json:"public_metrics"`
		} `json:"data"`
	}
	if err := gcall(ctx, tok, "GET", "https://api.twitter.com/2/users/"+me.Data.ID+"/tweets?max_results=20&exclude=retweets,replies&tweet.fields=public_metrics,created_at", nil, &tw); err == nil {
		for _, t := range tw.Data {
			when, _ := time.Parse(time.RFC3339, t.CreatedAt)
			snap.Posts = append(snap.Posts, socialPost{ID: t.ID, Text: t.Text, URL: "https://x.com/" + me.Data.Username + "/status/" + t.ID, When: when,
				Likes: t.PM.Like, Comments: t.PM.Reply, Shares: t.PM.Retweet + t.PM.Quote, Views: t.PM.Impr, HasViews: t.PM.Impr > 0})
		}
	}
	return snap, nil
}

func liGet(ctx context.Context, tok, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("LinkedIn-Version", "202405")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	return doJSON(req, out)
}

func fetchLinkedIn(ctx context.Context, tok string) (*socialSnap, error) {
	var ui struct{ Name, Email string }
	if err := liGet(ctx, tok, "https://api.linkedin.com/v2/userinfo", &ui); err != nil {
		return nil, err
	}
	partner := "LinkedIn is connected as " + ui.Name + ", but LinkedIn shares page analytics only with approved Marketing Developer Platform partners. Once your LinkedIn app has that approval, set LinkedIn partner access to 1 in Admin and reconnect."
	if env("LINKEDIN_PARTNER") != "1" {
		return nil, userErr(partner)
	}
	var acl struct {
		Elements []struct {
			Organization string `json:"organization"`
		} `json:"elements"`
	}
	if err := liGet(ctx, tok, "https://api.linkedin.com/rest/organizationAcls?q=roleAssignee&role=ADMINISTRATOR&state=APPROVED", &acl); err != nil || len(acl.Elements) == 0 {
		return nil, userErr(partner)
	}
	org := acl.Elements[0].Organization
	var ns struct {
		First float64 `json:"firstDegreeSize"`
	}
	if err := liGet(ctx, tok, "https://api.linkedin.com/rest/networkSizes/"+url.PathEscape(org)+"?edgeType=COMPANY_FOLLOWED_BY_MEMBER", &ns); err != nil {
		return nil, err
	}
	return &socialSnap{Platform: "linkedin", Handle: strings.TrimPrefix(org, "urn:li:organization:"), Followers: ns.First}, nil
}

func fetchPinterest(ctx context.Context, tok string) (*socialSnap, error) {
	var ua struct {
		Username  string  `json:"username"`
		About     string  `json:"about"`
		Followers float64 `json:"follower_count"`
		Following float64 `json:"following_count"`
	}
	if err := gcall(ctx, tok, "GET", "https://api.pinterest.com/v5/user_account", nil, &ua); err != nil {
		return nil, err
	}
	snap := &socialSnap{Platform: "pinterest", Handle: ua.Username, Bio: ua.About, Followers: ua.Followers, Following: ua.Following}
	var pins struct {
		Items []struct {
			ID, Title, Description, Link string
			CreatedAt                    string `json:"created_at"`
		} `json:"items"`
	}
	if err := gcall(ctx, tok, "GET", "https://api.pinterest.com/v5/pins?page_size=20", nil, &pins); err != nil {
		return snap, nil
	}
	end := time.Now().Format("2006-01-02")
	start := time.Now().AddDate(0, 0, -89).Format("2006-01-02")
	for i, p := range pins.Items {
		when, _ := time.Parse(time.RFC3339, p.CreatedAt)
		sp := socialPost{ID: p.ID, Text: firstNonEmptyStr(p.Title, p.Description), URL: "https://www.pinterest.com/pin/" + p.ID + "/", When: when}
		if i < 10 {
			var an struct {
				All struct {
					Summary map[string]float64 `json:"summary_metrics"`
				} `json:"all"`
			}
			if gcall(ctx, tok, "GET", "https://api.pinterest.com/v5/pins/"+p.ID+"/analytics?start_date="+start+"&end_date="+end+"&metric_types=IMPRESSION,SAVE,PIN_CLICK,OUTBOUND_CLICK", nil, &an) == nil {
				m := an.All.Summary
				sp.Views, sp.HasViews = m["IMPRESSION"], m["IMPRESSION"] > 0
				sp.Likes, sp.Comments, sp.Shares = m["SAVE"], m["PIN_CLICK"], m["OUTBOUND_CLICK"]
			}
		}
		snap.Posts = append(snap.Posts, sp)
	}
	return snap, nil
}

// provNeeds lists the developer keys still missing for a provider (empty when configured).
func provNeeds(p *provCfg) []map[string]any {
	var out []map[string]any
	if keyProv(p.ID) {
		return out
	}
	idl, sl := "Client ID", "Client secret"
	switch p.ID {
	case "pinterest", "threads":
		idl, sl = "App ID", "App secret"
	case "tiktok":
		idl = "Client key"
	case "instagram", "facebook":
		idl, sl = "App ID", "App secret"
	}
	if env(p.IDEnv) == "" {
		out = append(out, map[string]any{"key": p.IDEnv, "label": idl, "secret": false})
	}
	if env(p.SecretEnv) == "" {
		out = append(out, map[string]any{"key": p.SecretEnv, "label": sl, "secret": true})
	}
	if env("BASE_URL") == "" {
		out = append(out, map[string]any{"key": "BASE_URL", "label": "Public app URL", "secret": false})
	}
	return out
}

var _ = io.EOF
