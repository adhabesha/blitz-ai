package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Winning-examples research: finds the best-performing recent YouTube videos in the person's niche
// (real data: views, channel size, age), then has the AI explain what they share and write a script or
// design brief that follows their proven structure. Real performance data exists for YouTube only:
// TikTok, Instagram and Facebook have no public API for top content by topic.

type winVid struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Channel string   `json:"channel"`
	Thumb   string   `json:"thumb"`
	URL     string   `json:"url"`
	Views   int64    `json:"views"`
	Likes   int64    `json:"likes"`
	Comms   int64    `json:"comments"`
	Subs    int64    `json:"subs"`
	DurSec  float64  `json:"durSec"`
	AgeDays float64  `json:"ageDays"`
	PerDay  float64  `json:"perDay"`
	Ratio   float64  `json:"ratio"` // views divided by the channel's subscribers
	Why     string   `json:"why"`
	Tags    []string `json:"-"`
	desc    string
	chID    string
}

type winResult struct {
	at    time.Time
	vids  []*winVid
	tags  []string
	note  string
	label string
}

var winCache = struct {
	sync.Mutex
	m map[string]winResult
}{m: map[string]winResult{}}

func guessLang(s string) string {
	for _, r := range s {
		switch {
		case r >= 0x1200 && r <= 0x137F:
			return "am"
		case unicode.Is(unicode.Arabic, r):
			return "ar"
		case unicode.Is(unicode.Cyrillic, r):
			return "ru"
		case unicode.Is(unicode.Devanagari, r):
			return "hi"
		}
	}
	return ""
}

func atoi64(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }

func searchIDs(ctx context.Context, q, kind string, days int, order string) ([]string, error) {
	p := url.Values{"part": {"snippet"}, "type": {"video"}, "order": {order}, "maxResults": {"30"}, "q": {q},
		"publishedAfter": {time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)}}
	switch kind {
	case "short":
		p.Set("videoDuration", "short")
	case "video":
		p.Set("videoDuration", "medium")
	}
	if l := guessLang(q); l != "" {
		p.Set("relevanceLanguage", l)
	}
	var sr struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
		} `json:"items"`
	}
	if err := ytGet(ctx, "search", p, &sr); err != nil {
		return nil, err
	}
	var ids []string
	for _, it := range sr.Items {
		if it.ID.VideoID != "" {
			ids = append(ids, it.ID.VideoID)
		}
	}
	return ids, nil
}

// hydrate loads stats and channel sizes for new video ids into pool.
func hydrate(ctx context.Context, ids []string, kind string, pool map[string]*winVid, subs map[string]int64) error {
	var fresh []string
	for _, id := range ids {
		if _, ok := pool[id]; !ok {
			fresh = append(fresh, id)
			pool[id] = nil // placeholder so duplicates are skipped
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	var vr struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title        string   `json:"title"`
				Description  string   `json:"description"`
				PublishedAt  string   `json:"publishedAt"`
				ChannelID    string   `json:"channelId"`
				ChannelTitle string   `json:"channelTitle"`
				Tags         []string `json:"tags"`
			} `json:"snippet"`
			Statistics struct {
				Views    string `json:"viewCount"`
				Likes    string `json:"likeCount"`
				Comments string `json:"commentCount"`
			} `json:"statistics"`
			ContentDetails struct {
				Duration string `json:"duration"`
			} `json:"contentDetails"`
		} `json:"items"`
	}
	if err := ytGet(ctx, "videos", url.Values{"part": {"snippet,statistics,contentDetails"}, "id": {strings.Join(fresh, ",")}}, &vr); err != nil {
		return err
	}
	var needCh []string
	for _, it := range vr.Items {
		dur := parseDur(it.ContentDetails.Duration)
		if (kind == "short" && dur > 180) || (kind == "video" && dur <= 180) {
			continue
		}
		pub, _ := time.Parse(time.RFC3339, it.Snippet.PublishedAt)
		age := time.Since(pub).Hours() / 24
		if age < 1 {
			age = 1
		}
		v := &winVid{ID: it.ID, Title: it.Snippet.Title, Channel: it.Snippet.ChannelTitle, URL: "https://www.youtube.com/watch?v=" + it.ID,
			Thumb: "https://i.ytimg.com/vi/" + it.ID + "/mqdefault.jpg", Views: atoi64(it.Statistics.Views), Likes: atoi64(it.Statistics.Likes),
			Comms: atoi64(it.Statistics.Comments), DurSec: dur, AgeDays: age, Tags: it.Snippet.Tags, desc: it.Snippet.Description, chID: it.Snippet.ChannelID}
		v.PerDay = float64(v.Views) / age
		pool[it.ID] = v
		if _, ok := subs[v.chID]; !ok {
			subs[v.chID] = -1
			needCh = append(needCh, v.chID)
		}
	}
	for i := 0; i < len(needCh); i += 50 {
		j := i + 50
		if j > len(needCh) {
			j = len(needCh)
		}
		var cr ytChannelResp
		if ytGet(ctx, "channels", url.Values{"part": {"statistics"}, "id": {strings.Join(needCh[i:j], ",")}, "maxResults": {"50"}}, &cr) == nil {
			for _, c := range cr.Items {
				subs[c.ID] = atoi64(c.Statistics.Subs)
			}
		}
	}
	return nil
}

// selectTop ranks videos that reach far beyond their channel's size, limited per channel.
func selectTop(pool map[string]*winVid, subs map[string]int64, n int, minViews int64, perCh int) []*winVid {
	var vids []*winVid
	for _, v := range pool {
		if v == nil {
			continue
		}
		v.Subs = maxI64(subs[v.chID], 0)
		v.Ratio = float64(v.Views) / float64(maxI64(v.Subs, 500))
		vids = append(vids, v)
	}
	sort.Slice(vids, func(i, j int) bool {
		si, sj := vids[i].Ratio*float64(minI64(vids[i].Views, 50000)), vids[j].Ratio*float64(minI64(vids[j].Views, 50000))
		if si != sj {
			return si > sj
		}
		return vids[i].ID < vids[j].ID
	})
	var top []*winVid
	per := map[string]int{}
	for _, v := range vids {
		if v.Views < minViews || per[v.chID] >= perCh {
			continue
		}
		per[v.chID]++
		top = append(top, v)
		if len(top) >= n {
			break
		}
	}
	return top
}

// relatedQueries asks the AI for broader or alternative searches when a topic is too unusual to find examples directly.
func (s *server) relatedQueries(ctx context.Context, topic string) []string {
	txt, err := s.gemini(ctx, fmt.Sprintf(`A YouTube search for the topic below found too few videos. Return JSON only: {"queries":[3 YouTube search queries, each at most 6 words: (1) the broader parent subject in the same language as the topic, (2) an English search query for the same subject, (3) the most popular adjacent angle people search for]}. Topic: %s`, clip(topic, 200)))
	if err != nil {
		return nil
	}
	var q struct {
		Queries []string `json:"queries"`
	}
	if json.Unmarshal([]byte(cleanJSON(txt)), &q) != nil {
		return nil
	}
	var out []string
	for _, x := range q.Queries {
		if x = strings.TrimSpace(clip(x, 80)); x != "" && !strings.EqualFold(x, topic) && len(out) < 3 {
			out = append(out, x)
		}
	}
	return out
}

// findWinners widens the search step by step (longer time window, then related searches) so unusual topics still get examples.
func (s *server) findWinners(ctx context.Context, topic, kind string) (*winResult, error) {
	key := kind + "|" + strings.ToLower(topic)
	winCache.Lock()
	if e, ok := winCache.m[key]; ok && time.Since(e.at) < 6*time.Hour {
		winCache.Unlock()
		return &e, nil
	}
	winCache.Unlock()
	pool := map[string]*winVid{}
	subs := map[string]int64{}
	run := func(q string, days int, order string) error {
		ids, err := searchIDs(ctx, q, kind, days, order)
		if err != nil {
			return err
		}
		return hydrate(ctx, ids, kind, pool, subs)
	}
	label, note := "last 60 days", ""
	if err := run(topic, 60, "viewCount"); err != nil {
		return nil, err
	}
	top := selectTop(pool, subs, 8, 500, 2)
	if len(top) < 6 {
		label = "last 12 months"
		run(topic, 365, "viewCount")
		run(topic, 365, "relevance")
		top = selectTop(pool, subs, 8, 500, 2)
	}
	if len(top) < 6 {
		if rq := s.relatedQueries(ctx, topic); len(rq) > 0 {
			for _, q := range rq {
				run(q, 365, "viewCount")
			}
			note = "Few videos match your exact topic, so these also come from related searches: " + strings.Join(rq, " · ")
			top = selectTop(pool, subs, 8, 500, 2)
		}
	}
	if len(top) < 4 {
		top = selectTop(pool, subs, 8, 0, 3)
	}
	count := map[string]int{}
	for _, v := range top {
		seen := map[string]bool{}
		for _, t := range v.Tags {
			k := strings.ToLower(strings.Join(strings.Fields(strings.TrimPrefix(t, "#")), ""))
			if n := len([]rune(k)); n >= 3 && n <= 30 && !seen[k] {
				seen[k] = true
				count[k]++
			}
		}
		for _, t := range reHashInText.FindAllString(v.Title+" "+v.desc, -1) {
			k := strings.ToLower(strings.TrimPrefix(t, "#"))
			if !seen[k] {
				seen[k] = true
				count[k]++
			}
		}
	}
	type kv struct {
		k string
		n int
	}
	var all []kv
	for k, n := range count {
		all = append(all, kv{k, n})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].k < all[j].k
	})
	var tags []string
	for _, e := range all {
		if len(tags) >= 6 {
			break
		}
		tags = append(tags, e.k)
	}
	res := winResult{at: time.Now(), vids: top, tags: tags, note: note, label: label}
	winCache.Lock()
	winCache.m[key] = res
	winCache.Unlock()
	return &res, nil
}

// fetchThumbs downloads up to n thumbnails so the AI can look at the designs.
func fetchThumbs(ctx context.Context, vids []*winVid, n int) ([]string, [][]byte) {
	if len(vids) > n {
		vids = vids[:n]
	}
	type got struct {
		label string
		b     []byte
	}
	res := make([]got, len(vids))
	var wg sync.WaitGroup
	for i, v := range vids {
		wg.Add(1)
		go func(i int, v *winVid) {
			defer wg.Done()
			c, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			req, _ := http.NewRequestWithContext(c, http.MethodGet, v.Thumb, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return
			}
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 400<<10))
			if len(b) > 1000 {
				res[i] = got{"Image above is the thumbnail of id=" + v.ID + ":", b}
			}
		}(i, v)
	}
	wg.Wait()
	var labels []string
	var imgs [][]byte
	for _, g := range res {
		if g.b != nil {
			labels = append(labels, g.label)
			imgs = append(imgs, g.b)
		}
	}
	return labels, imgs
}

func maxI64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func minI64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

type winAnalysis struct {
	Patterns []string `json:"patterns"`
	PerVideo []struct {
		ID  string `json:"id"`
		Why string `json:"why"`
	} `json:"perVideo"`
	Titles []string `json:"titles"`
	Script struct {
		Format    string `json:"format"`
		LengthSec int    `json:"lengthSec"`
		Hook      string `json:"hook"`
		Beats     []struct {
			Time     string `json:"time"`
			Say      string `json:"say"`
			Show     string `json:"show"`
			OnScreen string `json:"onscreen"`
		} `json:"beats"`
		CTA     string `json:"cta"`
		Caption string `json:"caption"`
	} `json:"script"`
	Design struct {
		Format string `json:"format"`
		Slides []struct {
			Headline string `json:"headline"`
			Body     string `json:"body"`
			Visual   string `json:"visual"`
		} `json:"slides"`
		Layout     string   `json:"layout"`
		Text       string   `json:"text"`
		Colors     string   `json:"colors"`
		Elements   []string `json:"elements"`
		Variations []string `json:"variations"`
	} `json:"design"`
}

func (s *server) viralResearch(w http.ResponseWriter, r *http.Request) {
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
		Topic string `json:"topic"`
		Kind  string `json:"kind"` // short | video | image
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	in.Topic = strings.TrimSpace(clip(in.Topic, 200))
	if in.Kind != "short" && in.Kind != "video" && in.Kind != "image" {
		in.Kind = "short"
	}
	if in.Topic == "" {
		http.Error(w, "Type what the post is about in the Topic box first.", 400)
		return
	}
	if env("YOUTUBE_API_KEY") == "" {
		http.Error(w, "Add YOUTUBE_API_KEY in the admin console to find real top-performing examples.", 400)
		return
	}
	if !takeBudget(uid+":win", 10) {
		http.Error(w, "Daily limit for winning-example research reached (10). Try again tomorrow.", 429)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 80*time.Second)
	defer cancel()
	found, err := s.findWinners(ctx, in.Topic, in.Kind)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	vids, tags := found.vids, found.tags
	if len(vids) < 1 {
		http.Error(w, "Nothing found for that topic, even after widening the search. Try a broader topic or add a few English keywords.", 404)
		return
	}
	var sb strings.Builder
	for _, v := range vids {
		fmt.Fprintf(&sb, "- id=%s | title: %s | channel: %s (%d subscribers) | %d views in %.0f days (%.0f/day, %.1fx the channel size) | %.0f s | likes %d, comments %d | description start: %s\n",
			v.ID, clip(v.Title, 120), clip(v.Channel, 40), v.Subs, v.Views, v.AgeDays, v.PerDay, v.Ratio, v.DurSec, v.Likes, v.Comms, clip(strings.Join(strings.Fields(v.desc), " "), 160))
	}
	form := map[string]string{
		"short": "SHORT VIDEO (Reel/TikTok/Short, 30-45 seconds). Fill script: format=short, lengthSec, hook (the exact first spoken line, max 12 words), beats (6-9 items with time like 0-3s, say = exact words to speak, show = what the camera or screen shows, onscreen = the on-screen text), cta, caption (hook line, keyword line, call to action; no hashtags). Leave design with empty values.",
		"video": "NORMAL VIDEO (3-8 minutes). Fill script: format=video, lengthSec, hook (first 15 seconds spoken in full), beats (5-8 chapters with time ranges, say = the key lines or talking points in order, show = what is on screen, onscreen = on-screen text), cta, caption (video description opening, 2-3 lines). Fill design with a thumbnail concept (layout, text of max 4 words, colors, elements, variations).",
		"image": "IMAGE / GRAPHIC POST (for Instagram and Facebook). Leave script empty (format blank, no beats). Fill design: format (the best fit for this topic: single image, carousel, infographic or quote card), slides (for a carousel 5-7 items, otherwise 1 item: headline = the exact words, body = supporting text, visual = what to draw or photograph), layout (composition described precisely, based on what the winning thumbnails have in common), text (the exact words on the cover image, max 8 words), colors (named colours with contrast advice taken from the winners), elements (5 specific visual elements), variations (3 alternative concepts). Base the design on the winning thumbnails you can see and their titles.",
	}[in.Kind]
	prompt := fmt.Sprintf(`You are a short-form and YouTube content strategist. Below are the best-performing recent YouTube videos on the topic "%s", with real numbers. A ratio above 2x means the video reached far beyond what its channel normally gets, so its format is replicable.
%s
Task: work out what these winners share and create new content for the person's topic that follows the same proven structure WITHOUT copying any wording, scenes or thumbnails. Never invent facts, statistics, prices or claims about products. Write all spoken text, titles and on-screen text in the same language as the topic.
Deliverable type: %s
Return JSON only with keys: patterns (4-6 short, specific observations that appear in several winners: hook style, title formula, length, pacing, visual style, topic angle), perVideo (array of {id, why}: one sentence on why that video worked, using its numbers), titles (5 new title options following the winning title formulas), script (as described), design (as described).`,
		in.Topic, sb.String(), form)
	var txt string
	if in.Kind != "short" {
		// let the AI look at the winning thumbnails so the design advice is based on what they really look like
		if labels, imgs := fetchThumbs(ctx, vids, 6); len(imgs) >= 3 {
			txt, err = s.geminiImages(ctx, prompt+"\nThe thumbnails of the winning videos follow as images. Describe patterns you can actually see: composition, text size and placement, colours, faces or objects, contrast.", labels, imgs)
		}
	}
	if txt == "" {
		txt, err = s.gemini(ctx, prompt)
	}
	if errors.Is(err, errRate) {
		http.Error(w, "The free AI quota is busy right now. Try again in a minute.", 429)
		return
	}
	if err != nil {
		http.Error(w, "AI request failed. Try again.", 502)
		return
	}
	var an winAnalysis
	if json.Unmarshal([]byte(cleanJSON(txt)), &an) != nil {
		http.Error(w, "The AI reply could not be read. Try again.", 502)
		return
	}
	why := map[string]string{}
	for _, p := range an.PerVideo {
		why[p.ID] = p.Why
	}
	for _, v := range vids {
		v.Why = why[v.ID]
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"videos": vids, "analysis": an, "tags": tags, "source": "YouTube, " + found.label, "note": found.note})
}
