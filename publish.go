package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type pubSpec struct {
	Type, Title, Caption, Privacy, Path, Ext string
	Platforms                                []string
	Schedule                                 *time.Time
	Size                                     int64
	Dur                                      float64
	Notes                                    map[string]string
	FBReel                                   bool
}

type pubStatus struct {
	Platform string `json:"platform"`
	State    string `json:"state"` // queued | working | done | failed | skipped
	Message  string `json:"message"`
	URL      string `json:"url"`
}

type pubJob struct {
	mu      sync.Mutex
	UID     string
	Items   []*pubStatus
	Done    bool
	Created time.Time
}

func (j *pubJob) set(p, state, msg, link string) {
	j.mu.Lock()
	for _, it := range j.Items {
		if it.Platform == p {
			it.State, it.Message = state, msg
			if link != "" {
				it.URL = link
			}
		}
	}
	j.mu.Unlock()
}

var jobs = struct {
	sync.Mutex
	m map[string]*pubJob
}{m: map[string]*pubJob{}}

var (
	videoExt  = map[string]bool{".mp4": true, ".mov": true, ".m4v": true}
	imageExt  = map[string]bool{".jpg": true, ".jpeg": true, ".png": true}
	typePlats = map[string][]string{"short": {"youtube", "tiktok", "facebook", "instagram"}, "video": {"youtube", "facebook", "instagram"}, "image": {"facebook", "instagram"}}
	reMedia   = regexp.MustCompile(`^[0-9a-f]{32}\.[a-z0-9]{3,4}$`)
)

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func mediaDir() string {
	d := filepath.Join(os.TempDir(), "blitz-media")
	os.MkdirAll(d, 0o755)
	return d
}

func (sp pubSpec) mediaURL() string {
	return strings.TrimRight(env("BASE_URL"), "/") + "/media/" + filepath.Base(sp.Path)
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// publish receives one file plus text and fans it out to the chosen platforms.
func (s *server) publish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 320<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Upload not understood", 400)
		return
	}
	fields := map[string]string{}
	var path, ext string
	var size int64
	fail := func(msg string) {
		if path != "" {
			os.Remove(path)
		}
		http.Error(w, msg, 400)
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			fail("The upload was interrupted or is larger than 300 MB.")
			return
		}
		if part.FormName() == "file" && part.FileName() != "" {
			ext = strings.ToLower(filepath.Ext(part.FileName()))
			if !videoExt[ext] && !imageExt[ext] {
				fail("Use an MP4/MOV video or a JPG/PNG image.")
				return
			}
			path = filepath.Join(mediaDir(), randHex(16)+ext)
			f, err := os.Create(path)
			if err != nil {
				fail("Server could not store the file.")
				return
			}
			n, err := io.Copy(f, part)
			f.Close()
			if err != nil {
				fail("The upload was interrupted or is larger than 300 MB.")
				return
			}
			size = n
		} else {
			b, _ := io.ReadAll(io.LimitReader(part, 64<<10))
			fields[part.FormName()] = string(b)
		}
	}
	typ := fields["type"]
	allowed, ok := typePlats[typ]
	if !ok || path == "" {
		fail("Choose a post type and a file.")
		return
	}
	if (typ == "image") != imageExt[ext] {
		fail("The file doesn't match the post type (images need JPG/PNG, videos MP4/MOV).")
		return
	}
	sp := pubSpec{Type: typ, Title: strings.TrimSpace(fields["title"]), Caption: strings.TrimSpace(fields["caption"]), Privacy: fields["privacy"], Path: path, Ext: ext, Size: size}
	if sp.Privacy != "public" && sp.Privacy != "unlisted" {
		sp.Privacy = "private"
	}
	if v := fields["schedule"]; v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil || t.Before(time.Now().Add(10*time.Minute)) || t.After(time.Now().Add(75*24*time.Hour)) {
			fail("Pick a schedule time between 10 minutes and 75 days from now.")
			return
		}
		sp.Schedule = &t
	}
	seen := map[string]bool{}
	for _, p := range strings.Split(fields["platforms"], ",") {
		p = strings.TrimSpace(p)
		if has(allowed, p) && !seen[p] {
			seen[p] = true
			sp.Platforms = append(sp.Platforms, p)
		}
	}
	if len(sp.Platforms) == 0 {
		fail("Choose at least one platform that supports this post type.")
		return
	}
	var info *vidInfo
	var iw, ih int
	if typ == "image" {
		iw, ih = imageSize(path)
	} else {
		info, _ = probeMP4(path)
		if info != nil {
			sp.Dur = info.Dur
			sp.FBReel = info.W <= info.H && info.Dur >= 3 && info.Dur <= 90
		}
	}
	verd := preflight(sp, info, iw, ih)
	sp.Notes = map[string]string{}
	job := &pubJob{UID: uid, Created: time.Now()}
	runnable := 0
	var reasons []string
	for _, p := range sp.Platforms {
		it := &pubStatus{Platform: p, State: "queued"}
		v := verd[p]
		sp.Notes[p] = v.Note
		if v.Block != "" {
			it.State, it.Message = "failed", "Not posted: "+v.Block
			reasons = append(reasons, p+": "+v.Block)
		} else if sp.Schedule != nil && (p == "instagram" || p == "tiktok") {
			it.State, it.Message = "skipped", "Instagram and TikTok can't be scheduled through their APIs. Use Post now."
		} else {
			runnable++
		}
		job.Items = append(job.Items, it)
	}
	if runnable == 0 {
		os.Remove(path)
		msg := "Nothing could be posted: " + strings.Join(reasons, " | ")
		if len(reasons) == 0 {
			msg = "Nothing to post. Instagram and TikTok can't be scheduled."
		}
		http.Error(w, msg, 400)
		return
	}
	id := randHex(8)
	jobs.Lock()
	for k, j := range jobs.m {
		if time.Since(j.Created) > 3*time.Hour {
			delete(jobs.m, k)
		}
	}
	jobs.m[id] = job
	jobs.Unlock()
	go s.runPublish(job, uid, sp)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (s *server) publishStatus(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	jobs.Lock()
	j := jobs.m[r.URL.Query().Get("id")]
	jobs.Unlock()
	if j == nil || j.UID != uid {
		http.Error(w, "unknown job", 404)
		return
	}
	j.mu.Lock()
	out := map[string]any{"items": j.Items, "done": j.Done}
	b, _ := json.Marshal(out)
	j.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func (s *server) serveMedia(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/media/")
	if !reMedia.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(mediaDir(), name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func (s *server) runPublish(job *pubJob, uid string, sp pubSpec) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	var wg sync.WaitGroup
	for _, p := range sp.Platforms {
		p := p
		job.mu.Lock()
		skip := false
		for _, it := range job.Items {
			if it.Platform == p && (it.State == "skipped" || it.State == "failed") {
				skip = true
			}
		}
		job.mu.Unlock()
		if skip {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					log.Println("publish panic:", p, rec)
					job.set(p, "failed", "Internal error while publishing.", "")
				}
			}()
			job.set(p, "working", "Uploading...", "")
			var link, note string
			var err error
			switch p {
			case "youtube":
				link, note, err = s.pubYouTube(ctx, uid, sp)
			case "facebook":
				link, note, err = s.pubFacebook(ctx, uid, sp)
			case "instagram":
				link, note, err = s.pubInstagram(ctx, uid, sp)
			case "tiktok":
				link, note, err = s.pubTikTok(ctx, uid, sp)
			}
			if err != nil {
				log.Println("publish", p, err)
				job.set(p, "failed", trunc(err.Error(), 400), "")
				return
			}
			if n := sp.Notes[p]; n != "" {
				note += " " + n
			}
			job.set(p, "done", note, link)
		}()
	}
	wg.Wait()
	job.mu.Lock()
	job.Done = true
	job.mu.Unlock()
	time.AfterFunc(10*time.Minute, func() { os.Remove(sp.Path) })
}

// ---------------- YouTube ----------------

func (s *server) pubYouTube(ctx context.Context, uid string, sp pubSpec) (string, string, error) {
	if sp.Type == "image" {
		return "", "", fmt.Errorf("YouTube doesn't take images")
	}
	tok, err := s.accessToken(ctx, uid)
	if err != nil {
		return "", "", err
	}
	f, err := os.Open(sp.Path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	title := sp.Title
	if title == "" {
		title = strings.TrimSpace(strings.SplitN(sp.Caption, "\n", 2)[0])
	}
	if title == "" {
		title = "New video"
	}
	desc := sp.Caption
	if sp.Type == "short" && !strings.Contains(strings.ToLower(title+desc), "#shorts") {
		desc += "\n\n#Shorts"
	}
	status := map[string]any{"privacyStatus": sp.Privacy, "selfDeclaredMadeForKids": false}
	note := "Uploaded (" + sp.Privacy + ")"
	if sp.Schedule != nil {
		status["privacyStatus"] = "private"
		status["publishAt"] = sp.Schedule.UTC().Format(time.RFC3339)
		note = "Scheduled for " + sp.Schedule.Local().Format("2006-01-02 15:04") + ". YouTube publishes it."
	}
	meta, _ := json.Marshal(map[string]any{"snippet": map[string]any{"title": cut(title, 100), "description": cut(desc, 5000), "categoryId": "22"}, "status": status})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status", bytes.NewReader(meta))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(sp.Size, 10))
	req.Header.Set("X-Upload-Content-Type", "video/*")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	loc := resp.Header.Get("Location")
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
	resp.Body.Close()
	if resp.StatusCode != 200 || loc == "" {
		return "", "", fmt.Errorf("YouTube refused the upload (%d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	put, _ := http.NewRequestWithContext(ctx, http.MethodPut, loc, f)
	put.ContentLength = sp.Size
	put.Header.Set("Content-Type", "video/*")
	r2, err := http.DefaultClient.Do(put)
	if err != nil {
		return "", "", err
	}
	defer r2.Body.Close()
	if r2.StatusCode >= 300 {
		eb, _ := io.ReadAll(io.LimitReader(r2.Body, 300))
		return "", "", fmt.Errorf("YouTube upload failed (%d): %s", r2.StatusCode, strings.TrimSpace(string(eb)))
	}
	var out struct {
		ID string `json:"id"`
	}
	json.NewDecoder(r2.Body).Decode(&out)
	return "https://www.youtube.com/watch?v=" + out.ID, note, nil
}

// ---------------- Facebook ----------------

func postMultipart(ctx context.Context, u string, fields map[string]string, fileField, path string, out any) error {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		var werr error
		defer func() {
			mw.Close()
			pw.CloseWithError(werr)
		}()
		for k, v := range fields {
			if werr = mw.WriteField(k, v); werr != nil {
				return
			}
		}
		f, e := os.Open(path)
		if e != nil {
			werr = e
			return
		}
		defer f.Close()
		part, e := mw.CreateFormFile(fileField, filepath.Base(path))
		if e != nil {
			werr = e
			return
		}
		_, werr = io.Copy(part, f)
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return doJSON(req, out)
}

func (s *server) fbPage(ctx context.Context, uid string) (string, string, string, error) {
	t, err := s.validToken(ctx, uid, provOf("facebook"))
	if err != nil {
		return "", "", "", err
	}
	var acc struct {
		Data []struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Token string `json:"access_token"`
			User  string `json:"username"`
		} `json:"data"`
	}
	if err := getJSON(ctx, "https://graph.facebook.com/v21.0/me/accounts?"+url.Values{"fields": {"id,name,username,access_token"}, "access_token": {t.Access}}.Encode(), &acc); err != nil {
		return "", "", "", err
	}
	if len(acc.Data) == 0 {
		return "", "", "", userErr("No Facebook Page found. You must manage a Facebook Page to post to it.")
	}
	pick := 0
	if want := strings.ToLower(strings.TrimSpace(env("FACEBOOK_PAGE"))); want != "" {
		for i, p := range acc.Data {
			if strings.ToLower(p.User) == want || strings.ToLower(p.Name) == want {
				pick = i
				break
			}
		}
	}
	return acc.Data[pick].ID, acc.Data[pick].Token, acc.Data[pick].Name, nil
}

func (s *server) pubFacebook(ctx context.Context, uid string, sp pubSpec) (string, string, error) {
	pid, ptok, name, err := s.fbPage(ctx, uid)
	if err != nil {
		return "", "", err
	}
	caption := cut(sp.Caption, 5000)
	sched := map[string]string{}
	note := "Published on " + name
	if sp.Schedule != nil {
		sched["published"] = "false"
		sched["scheduled_publish_time"] = strconv.FormatInt(sp.Schedule.Unix(), 10)
		note = "Scheduled on " + name + " for " + sp.Schedule.Local().Format("2006-01-02 15:04")
	}
	if sp.Type == "image" {
		f := map[string]string{"access_token": ptok, "caption": caption}
		for k, v := range sched {
			f[k] = v
		}
		var out struct {
			ID     string `json:"id"`
			PostID string `json:"post_id"`
		}
		if err := postMultipart(ctx, "https://graph.facebook.com/v21.0/"+pid+"/photos", f, "source", sp.Path, &out); err != nil {
			return "", "", err
		}
		id := out.PostID
		if id == "" {
			id = out.ID
		}
		return "https://www.facebook.com/" + id, note, nil
	}
	if sp.Type == "short" && sp.Schedule == nil && sp.FBReel {
		link, rerr := s.fbReel(ctx, pid, ptok, caption, sp)
		if rerr == nil {
			return link, note + " as a Reel", nil
		}
		log.Println("fb reel failed, falling back to a normal video:", rerr)
		note += " (as a normal video; Reels publishing was refused: " + trunc(rerr.Error(), 140) + ")"
	}
	f := map[string]string{"access_token": ptok, "description": caption}
	if sp.Title != "" {
		f["title"] = cut(sp.Title, 100)
	}
	for k, v := range sched {
		f[k] = v
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := postMultipart(ctx, "https://graph-video.facebook.com/v21.0/"+pid+"/videos", f, "source", sp.Path, &out); err != nil {
		return "", "", err
	}
	return "https://www.facebook.com/watch/?v=" + out.ID, note, nil
}

func (s *server) fbReel(ctx context.Context, pid, ptok, caption string, sp pubSpec) (string, error) {
	base := "https://graph.facebook.com/v21.0/" + pid + "/video_reels"
	var st struct {
		VideoID   string `json:"video_id"`
		UploadURL string `json:"upload_url"`
	}
	if err := postForm(ctx, base, url.Values{"upload_phase": {"start"}, "access_token": {ptok}}, &st); err != nil {
		return "", err
	}
	if st.UploadURL == "" {
		return "", fmt.Errorf("no upload address returned")
	}
	f, err := os.Open(sp.Path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, st.UploadURL, f)
	req.ContentLength = sp.Size
	req.Header.Set("Authorization", "OAuth "+ptok)
	req.Header.Set("offset", "0")
	req.Header.Set("file_size", strconv.FormatInt(sp.Size, 10))
	var up map[string]any
	if err := doJSON(req, &up); err != nil {
		return "", err
	}
	var fin map[string]any
	if err := postForm(ctx, base, url.Values{"upload_phase": {"finish"}, "video_id": {st.VideoID}, "video_state": {"PUBLISHED"}, "description": {caption}, "access_token": {ptok}}, &fin); err != nil {
		return "", err
	}
	return "https://www.facebook.com/reel/" + st.VideoID, nil
}

// ---------------- Instagram ----------------

func (s *server) pubInstagram(ctx context.Context, uid string, sp pubSpec) (string, string, error) {
	if env("BASE_URL") == "" {
		return "", "", fmt.Errorf("BASE_URL is not set in the admin console; Instagram needs a public address for the media")
	}
	if sp.Type == "image" && sp.Ext == ".png" {
		return "", "", fmt.Errorf("Instagram only accepts JPG images through its API. Convert the image to JPG and try again")
	}
	t, err := s.validToken(ctx, uid, provOf("instagram"))
	if err != nil {
		return "", "", err
	}
	base := "https://graph.facebook.com/v21.0/"
	id, err := igAccount(ctx, t.Access)
	if err != nil {
		return "", "", err
	}
	form := url.Values{"access_token": {t.Access}, "caption": {cut(sp.Caption, 2200)}}
	if sp.Type == "image" {
		form.Set("image_url", sp.mediaURL())
	} else {
		form.Set("media_type", "REELS")
		form.Set("video_url", sp.mediaURL())
		form.Set("share_to_feed", "true")
	}
	var c struct {
		ID string `json:"id"`
	}
	if err := postForm(ctx, base+id+"/media", form, &c); err != nil {
		return "", "", fmt.Errorf("Instagram refused the media (is instagram_content_publish in your configuration, and did you reconnect Instagram?): %s", trunc(err.Error(), 200))
	}
	if c.ID == "" {
		return "", "", fmt.Errorf("Instagram returned no media container")
	}
	ready := false
	for i := 0; i < 72; i++ {
		var st struct {
			Code string `json:"status_code"`
			Info string `json:"status"`
		}
		if getJSON(ctx, base+c.ID+"?"+url.Values{"fields": {"status_code,status"}, "access_token": {t.Access}}.Encode(), &st) == nil {
			if st.Code == "FINISHED" {
				ready = true
				break
			}
			if st.Code == "ERROR" || st.Code == "EXPIRED" {
				return "", "", fmt.Errorf("Instagram could not process the file: %s", st.Info)
			}
		}
		time.Sleep(5 * time.Second)
	}
	if !ready {
		return "", "", fmt.Errorf("Instagram is still processing the video; it took longer than 6 minutes")
	}
	var pub struct {
		ID string `json:"id"`
	}
	if err := postForm(ctx, base+id+"/media_publish", url.Values{"creation_id": {c.ID}, "access_token": {t.Access}}, &pub); err != nil {
		return "", "", err
	}
	link := "https://www.instagram.com/"
	var pl struct {
		Permalink string `json:"permalink"`
	}
	if getJSON(ctx, base+pub.ID+"?"+url.Values{"fields": {"permalink"}, "access_token": {t.Access}}.Encode(), &pl) == nil && pl.Permalink != "" {
		link = pl.Permalink
	}
	return link, "Published on Instagram", nil
}

// ---------------- TikTok ----------------

func (s *server) pubTikTok(ctx context.Context, uid string, sp pubSpec) (string, string, error) {
	if sp.Type == "image" {
		return "", "", fmt.Errorf("TikTok image posts aren't supported here")
	}
	t, err := s.validToken(ctx, uid, provOf("tiktok"))
	if err != nil {
		return "", "", err
	}
	var ci struct {
		Data struct {
			Opts   []string `json:"privacy_level_options"`
			MaxDur float64  `json:"max_video_post_duration_sec"`
		} `json:"data"`
	}
	if err := gcall(ctx, t.Access, "POST", "https://open.tiktokapis.com/v2/post/publish/creator_info/query/", map[string]any{}, &ci); err != nil {
		if strings.Contains(err.Error(), "scope_not_authorized") {
			return "", "", fmt.Errorf("TikTok is connected without posting permission. In Connected social click Disconnect on TikTok, then Connect again and approve video posting")
		}
		return "", "", fmt.Errorf("TikTok refused the request (is Content Posting API with video.publish enabled, and did you reconnect TikTok?): %s", trunc(err.Error(), 160))
	}
	if ci.Data.MaxDur > 0 && sp.Dur > ci.Data.MaxDur {
		return "", "", fmt.Errorf("This video is %.0f s but your TikTok account allows at most %.0f s", sp.Dur, ci.Data.MaxDur)
	}
	priv := "SELF_ONLY"
	if has(ci.Data.Opts, "PUBLIC_TO_EVERYONE") {
		priv = "PUBLIC_TO_EVERYONE"
	} else if len(ci.Data.Opts) > 0 && !has(ci.Data.Opts, "SELF_ONLY") {
		priv = ci.Data.Opts[0]
	}
	size := sp.Size
	chunk := int64(10 << 20)
	chunkSize, total := size, int64(1)
	if size > chunk {
		chunkSize, total = chunk, size/chunk
	}
	initBody := map[string]any{
		"post_info":   map[string]any{"title": cut(sp.Caption, 2200), "privacy_level": priv, "disable_duet": false, "disable_comment": false, "disable_stitch": false},
		"source_info": map[string]any{"source": "FILE_UPLOAD", "video_size": size, "chunk_size": chunkSize, "total_chunk_count": total},
	}
	var ir struct {
		Data struct {
			PublishID string `json:"publish_id"`
			UploadURL string `json:"upload_url"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := gcall(ctx, t.Access, "POST", "https://open.tiktokapis.com/v2/post/publish/video/init/", initBody, &ir); err != nil {
		return "", "", err
	}
	if ir.Data.UploadURL == "" {
		return "", "", fmt.Errorf("TikTok gave no upload address: %s %s", ir.Error.Code, ir.Error.Message)
	}
	f, err := os.Open(sp.Path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	ctype := "video/mp4"
	if sp.Ext == ".mov" {
		ctype = "video/quicktime"
	}
	for i := int64(0); i < total; i++ {
		start := i * chunkSize
		end := start + chunkSize - 1
		if i == total-1 {
			end = size - 1
		}
		n := end - start + 1
		req, _ := http.NewRequestWithContext(ctx, http.MethodPut, ir.Data.UploadURL, io.NewSectionReader(f, start, n))
		req.ContentLength = n
		req.Header.Set("Content-Type", ctype)
		req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", "", err
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			return "", "", fmt.Errorf("TikTok upload failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(b)))
		}
	}
	note := "Sent to TikTok"
	if priv == "SELF_ONLY" {
		note = "Sent to TikTok as PRIVATE (TikTok keeps posts from unaudited apps private)"
	}
	for i := 0; i < 40; i++ {
		var st struct {
			Data struct {
				Status string `json:"status"`
				Reason string `json:"fail_reason"`
			} `json:"data"`
		}
		if gcall(ctx, t.Access, "POST", "https://open.tiktokapis.com/v2/post/publish/status/fetch/", map[string]any{"publish_id": ir.Data.PublishID}, &st) == nil {
			switch st.Data.Status {
			case "PUBLISH_COMPLETE":
				return "https://www.tiktok.com/", note, nil
			case "FAILED":
				return "", "", fmt.Errorf("TikTok failed to publish: %s", st.Data.Reason)
			}
		}
		time.Sleep(5 * time.Second)
	}
	return "https://www.tiktok.com/", note + " (still processing)", nil
}
