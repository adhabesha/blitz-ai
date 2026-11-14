package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

// "Apply" backends: the website connection (a GitHub repo that deploys the site) and the YouTube channel profile.

var (
	reRepo   = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)
	reBranch = regexp.MustCompile(`^[A-Za-z0-9_./-]{1,100}$`)
	reHost   = regexp.MustCompile(`^[a-z0-9.-]{3,253}$`)
)

type siteCfg struct {
	Repo, Branch, Token, HTML, Domain string
}

func (s *server) siteLoad(ctx context.Context, uid string) (*siteCfg, error) {
	d, err := s.db.Collection("users").Doc(uid).Collection("private").Doc("site_github").Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("Connect your website first")
	}
	m := d.Data()
	str := func(k string) string { v, _ := m[k].(string); return v }
	c := &siteCfg{Repo: str("repo"), Branch: str("branch"), Token: str("token"), HTML: str("html"), Domain: str("domain")}
	if c.Repo == "" || c.Token == "" {
		return nil, fmt.Errorf("Connect your website first")
	}
	return c, nil
}

func ghDo(ctx context.Context, token, method, p string, body any, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+p, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "blitz-ai")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		json.Unmarshal(b, &e)
		return resp.StatusCode, fmt.Errorf("GitHub %d: %s", resp.StatusCode, trunc(e.Message, 200))
	}
	if out != nil {
		json.Unmarshal(b, out)
	}
	return resp.StatusCode, nil
}

func ghGetFile(ctx context.Context, c *siteCfg, file string) (content, sha string, found bool, err error) {
	var r struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	code, err := ghDo(ctx, c.Token, "GET", "/repos/"+c.Repo+"/contents/"+escPath(file)+"?ref="+url.QueryEscape(c.Branch), nil, &r)
	if code == 404 {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	if r.Encoding != "base64" {
		return "", "", false, fmt.Errorf("%s is too large to edit here", file)
	}
	b, derr := base64.StdEncoding.DecodeString(strings.ReplaceAll(r.Content, "\n", ""))
	if derr != nil {
		return "", "", false, derr
	}
	return string(b), r.SHA, true, nil
}

func ghPutFile(ctx context.Context, c *siteCfg, file, content, sha, msg string) (string, error) {
	body := map[string]any{"message": msg, "content": base64.StdEncoding.EncodeToString([]byte(content)), "branch": c.Branch}
	if sha != "" {
		body["sha"] = sha
	}
	var r struct {
		Commit struct {
			URL string `json:"html_url"`
		} `json:"commit"`
	}
	if _, err := ghDo(ctx, c.Token, "PUT", "/repos/"+c.Repo+"/contents/"+escPath(file), body, &r); err != nil {
		return "", err
	}
	return r.Commit.URL, nil
}

func escPath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

func cleanHTMLPath(p string) (string, bool) {
	p = strings.TrimPrefix(strings.TrimSpace(p), "/")
	if p == "" {
		p = "index.html"
	}
	if strings.Contains(p, "..") || strings.Contains(p, "\\") || len(p) > 200 || !strings.HasSuffix(strings.ToLower(p), ".html") {
		return "", false
	}
	return path.Clean(p), true
}

func (s *server) siteStatus(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	out := map[string]any{"connected": false}
	if c, err := s.siteLoad(r.Context(), uid); err == nil {
		out = map[string]any{"connected": true, "repo": c.Repo, "branch": c.Branch, "html": c.HTML, "domain": c.Domain}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *server) siteConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct{ Repo, Branch, Token, HTML, Domain string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	in.Repo = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(in.Repo), "https://github.com/"), "github.com/"))
	in.Repo = strings.TrimSuffix(in.Repo, ".git")
	in.Branch = strings.TrimSpace(in.Branch)
	if in.Branch == "" {
		in.Branch = "main"
	}
	in.Token = strings.TrimSpace(in.Token)
	dom := strings.ToLower(strings.TrimSpace(in.Domain))
	dom = strings.TrimPrefix(strings.TrimPrefix(dom, "https://"), "http://")
	dom = strings.TrimPrefix(strings.SplitN(dom, "/", 2)[0], "www.")
	html, ok := cleanHTMLPath(in.HTML)
	switch {
	case !reRepo.MatchString(in.Repo):
		http.Error(w, "Repository must look like owner/name, for example adhabesha/my-site.", 400)
		return
	case !reBranch.MatchString(in.Branch) || strings.Contains(in.Branch, ".."):
		http.Error(w, "That branch name isn't valid.", 400)
		return
	case in.Token == "" || len(in.Token) > 300:
		http.Error(w, "Paste a GitHub token (see the steps above the box).", 400)
		return
	case !ok:
		http.Error(w, "The homepage file must be an .html path such as index.html or docs/index.html.", 400)
		return
	case !reHost.MatchString(dom) || !strings.Contains(dom, "."):
		http.Error(w, "Enter your website address, for example mysite.com.", 400)
		return
	}
	c := &siteCfg{Repo: in.Repo, Branch: in.Branch, Token: in.Token, HTML: html, Domain: dom}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var rp struct {
		Perm struct {
			Push bool `json:"push"`
		} `json:"permissions"`
	}
	if code, err := ghDo(ctx, c.Token, "GET", "/repos/"+c.Repo, nil, &rp); err != nil {
		if code == 404 || code == 401 || code == 403 {
			http.Error(w, "GitHub can't see that repository with this token. Check the name and that the token has access to it.", 400)
		} else {
			http.Error(w, err.Error(), 400)
		}
		return
	}
	if _, _, found, err := ghGetFile(ctx, c, html); err != nil {
		http.Error(w, err.Error(), 400)
		return
	} else if !found {
		http.Error(w, "I can't find "+html+" on branch "+c.Branch+". Fix the homepage file path or the branch.", 400)
		return
	}
	_, err = s.db.Collection("users").Doc(uid).Collection("private").Doc("site_github").Set(ctx, map[string]any{
		"repo": c.Repo, "branch": c.Branch, "token": c.Token, "html": c.HTML, "domain": c.Domain, "updatedAt": time.Now(),
	})
	if err != nil {
		http.Error(w, "Could not save. Try again.", 500)
		return
	}
	w.Write([]byte(`{"ok":true}`))
}

func (s *server) siteDisconnect(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil || r.Method != http.MethodPost {
		http.Error(w, "unauthorized", 401)
		return
	}
	s.db.Collection("users").Doc(uid).Collection("private").Doc("site_github").Delete(r.Context())
	w.Write([]byte(`{"ok":true}`))
}

var siteFiles = map[string]bool{"robots.txt": true, "sitemap.xml": true, "llms.txt": true}

// siteApply writes a file or patches the homepage <head> in the connected repository (one commit per apply).
func (s *server) siteApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		Kind      string   `json:"kind"`
		File      string   `json:"file"`
		Content   string   `json:"content"`
		Overwrite bool     `json:"overwrite"`
		Ops       []headOp `json:"ops"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	c, err := s.siteLoad(ctx, uid)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	reply := func(msg, link string) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": msg, "url": link})
	}
	dir := path.Dir(c.HTML)
	switch in.Kind {
	case "file":
		if !siteFiles[in.File] || strings.TrimSpace(in.Content) == "" || len(in.Content) > 200<<10 {
			http.Error(w, "That file can't be written.", 400)
			return
		}
		target := in.File
		if dir != "." {
			target = dir + "/" + in.File
		}
		cur, sha, found, err := ghGetFile(ctx, c, target)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if found && cur == in.Content {
			reply(in.File+" is already up to date.", "")
			return
		}
		if found && !in.Overwrite {
			http.Error(w, "EXISTS", 409)
			return
		}
		link, err := ghPutFile(ctx, c, target, in.Content, sha, "ficx: "+ifs(found, "update ", "add ")+in.File)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		reply("Done. "+in.File+" was committed to "+c.Repo+". Your host deploys it in a minute or two.", link)
	case "head":
		if len(in.Ops) == 0 || len(in.Ops) > 20 {
			http.Error(w, "Nothing to change.", 400)
			return
		}
		cur, sha, found, err := ghGetFile(ctx, c, c.HTML)
		if err != nil || !found {
			http.Error(w, ifs(err != nil, "Could not read your homepage file.", c.HTML+" was not found."), 400)
			return
		}
		out, changed, err := patchHead(cur, in.Ops)
		if err != nil {
			http.Error(w, "Not applied: "+err.Error(), 400)
			return
		}
		if out == cur {
			reply("Your page already has this. Nothing to change.", "")
			return
		}
		link, err := ghPutFile(ctx, c, c.HTML, out, sha, "ficx: update "+strings.Join(changed, ", "))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		reply("Done. Updated "+strings.Join(changed, ", ")+" in "+c.HTML+". Your host deploys it in a minute or two. To undo, revert the commit on GitHub.", link)
	default:
		http.Error(w, "bad request", 400)
	}
}

// applyYouTubeChannel updates the channel description and keywords.
func (s *server) applyYouTubeChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		Description string   `json:"description"`
		Keywords    []string `json:"keywords"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	in.Description = strings.TrimSpace(in.Description)
	if in.Description == "" || len([]rune(in.Description)) > 1000 {
		http.Error(w, "The channel description must be 1-1000 characters.", 400)
		return
	}
	tok, err := s.accessToken(r.Context(), uid)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var cur struct {
		Items []map[string]any `json:"items"`
	}
	if err := gcall(ctx, tok, "GET", "https://www.googleapis.com/youtube/v3/channels?part=brandingSettings&mine=true", nil, &cur); err != nil || len(cur.Items) == 0 {
		http.Error(w, "Could not read your YouTube channel. Connect Google and allow YouTube access.", 400)
		return
	}
	item := cur.Items[0]
	bs, _ := item["brandingSettings"].(map[string]any)
	if bs == nil {
		bs = map[string]any{}
	}
	ch, _ := bs["channel"].(map[string]any)
	if ch == nil {
		ch = map[string]any{}
	}
	ch["description"] = in.Description
	var kw []string
	total := 0
	for _, k := range in.Keywords {
		k = strings.TrimSpace(strings.TrimPrefix(k, "#"))
		if k == "" || total+len(k)+3 > 480 {
			continue
		}
		total += len(k) + 3
		if strings.Contains(k, " ") {
			k = `"` + strings.ReplaceAll(k, `"`, "") + `"`
		}
		kw = append(kw, k)
	}
	if len(kw) > 0 {
		ch["keywords"] = strings.Join(kw, " ")
	}
	bs["channel"] = ch
	if err := gcall(ctx, tok, "PUT", "https://www.googleapis.com/youtube/v3/channels?part=brandingSettings", map[string]any{"id": item["id"], "brandingSettings": bs}, nil); err != nil {
		http.Error(w, "YouTube rejected the update: "+trunc(err.Error(), 160), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

var reFBPostID = regexp.MustCompile(`^(\d{5,30})_(\d{5,30})$`)

// applyFacebookPost edits the caption of a post that is already live on the connected Facebook Page.
// Instagram and TikTok don't let any app change a published caption, so they are refused with a clear reason.
func (s *server) applyPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		Provider string `json:"provider"`
		PostID   string `json:"postId"`
		Message  string `json:"message"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	in.Message = strings.TrimSpace(in.Message)
	switch {
	case in.Provider == "instagram" || in.Provider == "tiktok":
		http.Error(w, strings.Title(in.Provider)+" does not let any app edit a post that is already published. Copy the new caption, open the post and use Edit caption there.", 400)
		return
	case in.Provider != "facebook":
		http.Error(w, "bad request", 400)
		return
	case in.Message == "" || len([]rune(in.Message)) > 5000:
		http.Error(w, "The caption must be 1-5000 characters.", 400)
		return
	}
	m := reFBPostID.FindStringSubmatch(in.PostID)
	if m == nil {
		http.Error(w, "That post can't be edited from here (unsupported post id).", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	t, err := s.validToken(ctx, uid, provOf("facebook"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// The post id starts with its Page id, so the Page token comes straight from that Page.
	var pg struct {
		Token string `json:"access_token"`
	}
	if err := getJSON(ctx, "https://graph.facebook.com/v21.0/"+m[1]+"?"+url.Values{"fields": {"access_token"}, "access_token": {t.Access}}.Encode(), &pg); err != nil || pg.Token == "" {
		http.Error(w, "Could not get permission to edit this Page. Disconnect and reconnect Facebook, and tick your Page on Meta's screen.", 400)
		return
	}
	var out struct {
		Success bool `json:"success"`
	}
	if err := postForm(ctx, "https://graph.facebook.com/v21.0/"+in.PostID, url.Values{"message": {in.Message}, "access_token": {pg.Token}}, &out); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "(#200)") || strings.Contains(msg, "pages_manage_posts") || strings.Contains(msg, "permission"):
			http.Error(w, "Facebook refused the edit: ficx needs the pages_manage_posts permission. Add it in your Meta configuration (SETUP-SOCIAL.md), then Disconnect and Connect Facebook again.", 400)
		default:
			http.Error(w, "Facebook did not accept the edit (it only allows changing the text of some post types): "+trunc(msg, 180), 400)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Done. The caption of this post on your Facebook Page was updated.", "url": "https://www.facebook.com/" + in.PostID})
}
