package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var budget = struct {
	sync.Mutex
	m map[string]int
}{m: map[string]int{}}

func takeBudget(uid string, max int) bool {
	k := uid + time.Now().UTC().Format("2006-01-02")
	budget.Lock()
	defer budget.Unlock()
	if budget.m[k] >= max {
		return false
	}
	budget.m[k]++
	return true
}

var platformRules = map[string]string{
	"youtube":   "Built for virality. Titles under 70 characters, main keyword first, one curiosity or benefit hook, optionally a number. The first 150 characters of the description contain the main keyword and a hook; description 800-1500 characters with a call to action and placeholders for links and timestamps. Tags in tiers: 3 broad (high-volume), 5 niche (the exact topic), 6 long-tail (3-5 word searches people really type), 4 question tags (how/why/what), 3 trending-style tags for the topic; total under 450 characters, no misleading tags. 3 relevant hashtags. Reuse words and tags that already perform on this channel when they fit. Thumbnail ideas with 2-4 word overlay text.",
	"instagram": "Caption first line is a hook under 125 characters, then value, then one call to action. 3-8 relevant hashtags mixing niche and mid-size. Under 2200 characters.",
	"tiktok":    "Caption under 150 characters with a hook and 3-5 hashtags. Hooks must work in the first 3 seconds of video and on-screen text.",
	"x":         "Posts under 260 characters, one idea, strong first words, at most 2 hashtags, an invitation to reply.",
	"linkedin":  "A hook in the first line (before the 'see more' cut), short paragraphs, a concrete lesson or story, a question at the end, 3 hashtags.",
	"facebook":  "A conversational opener, 1-3 short paragraphs, a clear call to action, at most 3 hashtags.",
	"app":       "App store listing: title under 30 characters (brand + one keyword), a 30-character subtitle, a 100-character keyword list, and a description whose first 3 lines sell the main benefit, then features, social proof and a call to action.",
	"post":      "Cross-platform short video for TikTok, Instagram Reels, YouTube Shorts and Facebook. Caption: the first line is a hook under 90 characters (a curiosity gap, a bold claim, a number or a direct question to the viewer); the second line states the main keyword in plain words, because search matters on all four platforms; then one short value line; end with one call to action that asks for a comment, share or save (not 'like and follow'). Under 300 characters before the hashtags. Hooks: 3 opening lines for the first 3 seconds of the video and its on-screen text. Ideas: 3 cover or first-frame ideas. Notes: 3 specific things to test on the next post.",
	"bio":       "Give 5 bio options under 150 characters: who you help, the result they get, one call to action and a prompt to use the link. No emojis spam, no invented facts.",
	"ideas":     "Give 8 specific content ideas for the next 4 weeks. Each idea is a ready-to-use title or hook. Mix educational, story, opinion and proof content, and build on the topic that already works.",
	"howto":     "Turn the task into numbered, concrete implementation steps a beginner can follow today with free tools. Add 3 ready-to-use examples or templates.",
	"faq":       "Write 5 questions real buyers ask about this business and a concise, factual 2-sentence answer for each, in the same order. Do not invent prices, awards or statistics.",
	"website":   "SEO title under 60 characters with the main keyword first, meta description 140-155 characters with a benefit and call to action, plus one H1 suggestion in the notes.",
}

type optIn struct {
	Platform     string   `json:"platform"`
	VideoID      string   `json:"videoId"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	Topic        string   `json:"topic"`
	Audience     string   `json:"audience"`
	Goal         string   `json:"goal"`
	HashtagCount int      `json:"hashtagCount"`
}

type optOut struct {
	Titles      []string            `json:"titles"`
	Description string              `json:"description"`
	Tags        []string            `json:"tags"`
	Hashtags    []string            `json:"hashtags"`
	Hooks       []string            `json:"hooks"`
	Ideas       []string            `json:"ideas"`
	Notes       []string            `json:"notes"`
	TagTiers    map[string][]string `json:"tagTiers"`
}

type trendEnt struct {
	at   time.Time
	tags []string
}

var trendCache = struct {
	sync.Mutex
	m map[string]trendEnt
}{m: map[string]trendEnt{}}

var reHashInText = regexp.MustCompile(`#[\p{L}\p{N}_]{3,30}`)

// ytTopTags returns the tags and hashtags most used by the most-viewed recent YouTube videos on a topic.
// It is real data (what tools like vidIQ and TubeBuddy show), cached for 6 hours to save API quota.
func ytTopTags(ctx context.Context, topic string) []string {
	topic = strings.ToLower(strings.TrimSpace(topic))
	if topic == "" || env("YOUTUBE_API_KEY") == "" {
		return nil
	}
	trendCache.Lock()
	if e, ok := trendCache.m[topic]; ok && time.Since(e.at) < 6*time.Hour {
		trendCache.Unlock()
		return e.tags
	}
	trendCache.Unlock()
	var sr struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
		} `json:"items"`
	}
	after := time.Now().AddDate(0, 0, -90).UTC().Format(time.RFC3339)
	if ytGet(ctx, "search", url.Values{"part": {"snippet"}, "type": {"video"}, "order": {"viewCount"}, "publishedAfter": {after}, "maxResults": {"12"}, "q": {topic}}, &sr) != nil {
		return nil
	}
	var ids []string
	for _, it := range sr.Items {
		if it.ID.VideoID != "" {
			ids = append(ids, it.ID.VideoID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var vr ytVideosResp
	if ytGet(ctx, "videos", url.Values{"part": {"snippet"}, "id": {strings.Join(ids, ",")}}, &vr) != nil {
		return nil
	}
	count := map[string]int{}
	norm := func(t string) string {
		t = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		t = strings.Join(strings.Fields(t), "")
		if n := len([]rune(t)); n < 3 || n > 30 {
			return ""
		}
		return t
	}
	for _, v := range vr.Items {
		seen := map[string]bool{}
		for _, t := range v.Snippet.Tags {
			if k := norm(t); k != "" && !seen[k] {
				seen[k] = true
				count[k]++
			}
		}
		for _, t := range reHashInText.FindAllString(v.Snippet.Title+" "+v.Snippet.Description, -1) {
			if k := norm(t); k != "" && !seen[k] {
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
	var out []string
	for _, e := range all {
		if len(out) >= 25 {
			break
		}
		out = append(out, "#"+e.k)
	}
	trendCache.Lock()
	trendCache.m[topic] = trendEnt{time.Now(), out}
	trendCache.Unlock()
	return out
}

func (s *server) optimize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in optIn
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	pl := strings.ToLower(in.Platform)
	rules, ok := platformRules[pl]
	if !ok {
		http.Error(w, "Choose a platform", 400)
		return
	}
	title, desc, tags := in.Title, in.Description, in.Tags
	if in.VideoID != "" && pl == "youtube" && title == "" && env("YOUTUBE_API_KEY") != "" {
		var vr ytVideosResp
		if err := ytGet(r.Context(), "videos", url.Values{"part": {"snippet"}, "id": {in.VideoID}}, &vr); err == nil && len(vr.Items) > 0 {
			title, desc, tags = vr.Items[0].Snippet.Title, vr.Items[0].Snippet.Description, vr.Items[0].Snippet.Tags
		}
	}
	if strings.TrimSpace(title+desc+in.Topic) == "" {
		http.Error(w, "Add a topic, a title or a description first", 400)
		return
	}
	if !takeBudget(uid, 25) {
		http.Error(w, "Daily optimizer limit reached (25). Try again tomorrow.", 429)
		return
	}
	n := 5
	if pl == "ideas" {
		n = 8
	}
	label := pl
	if pl == "post" {
		label = "short-form social video"
	}
	var real []string
	if pl == "post" || pl == "youtube" || pl == "instagram" || pl == "tiktok" {
		tctx, tcancel := context.WithTimeout(r.Context(), 12*time.Second)
		real = ytTopTags(tctx, firstNonEmpty(in.Topic, title))
		tcancel()
	}
	extra := ""
	if len(real) > 0 {
		extra += "\nReal data: tags and hashtags used by the most-viewed recent YouTube videos on this topic, most common first (use the ones that truly fit the post, ignore unrelated ones): " + strings.Join(real, " ") + "."
	}
	if in.HashtagCount >= 3 {
		if in.HashtagCount > 20 {
			in.HashtagCount = 20
		}
		extra += fmt.Sprintf("\nHashtags: return exactly %d in hashtags, built as a pyramid: broad 2-3 (very popular, topic level), mid 3-4, niche 3-5 (the exact subject), micro 2-3 (very specific long tags), trending 1-2 (tied to a format, season or event you are confident is current, otherwise leave out). Merge in that order and trim to the count. No spammy, banned or misleading tags, no duplicates, no spaces, no punctuation.", in.HashtagCount)
	}
	prompt := fmt.Sprintf("You are an expert %s growth strategist. Optimise the content below for maximum reach and clicks using the platform's published best practices, and never invent facts about the creator.\nRules: %s\nCurrent title: %s\nCurrent description or caption: %s\nCurrent tags: %s\nTopic: %s\nAudience: %s\nGoal: %s\nReturn JSON only with keys: titles (%d distinct options; for non-video platforms caption openers; for ideas the content ideas; for faq the questions; for bio the bio options), description (the final improved full description, caption or bio, ready to paste; for howto the numbered steps; for faq a one-line summary), tags (keywords without #; empty list if the platform doesn't use tags), hashtags (with #), hooks (3 strong opening lines; for faq the matching answers in the same order as titles; for howto 3 ready-to-use examples), ideas (3 thumbnail or visual ideas), notes (3 short A/B testing tips), tagTiers (an object with array keys. For youtube: broad, niche, longtail, questions, trending. For every other platform: the hashtag tiers broad, mid, niche, micro, trending).%s",
		label, rules, clip(title, 150), clip(desc, 1500), clip(strings.Join(tags, ", "), 400), clip(in.Topic, 400), clip(in.Audience, 150), clip(in.Goal, 150), n, extra)
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	txt, err := s.gemini(ctx, prompt)
	if errors.Is(err, errRate) {
		http.Error(w, "The free AI quota is busy right now. Try again in a minute.", 429)
		return
	}
	if err != nil {
		http.Error(w, "AI request failed. Try again.", 502)
		return
	}
	var out optOut
	if json.Unmarshal([]byte(cleanJSON(txt)), &out) != nil || out.Description == "" {
		http.Error(w, "The AI reply could not be read. Try again.", 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"result": out, "current": map[string]any{"title": title, "description": desc, "tags": tags}, "sourceTags": real})
}

var reVideoID = regexp.MustCompile(`^[\w-]{6,20}$`)

type applyIn struct {
	VideoID     string   `json:"videoId"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

// applyYouTube updates title, description and tags of one of the user's own videos.
func (s *server) applyYouTube(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in applyIn
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil || !reVideoID.MatchString(in.VideoID) {
		http.Error(w, "bad request", 400)
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" || len([]rune(in.Title)) > 100 || len([]rune(in.Description)) > 5000 {
		http.Error(w, "Title must be 1-100 characters and the description under 5000.", 400)
		return
	}
	var tags []string
	total := 0
	for _, t := range in.Tags {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		if t == "" || total+len(t) > 450 {
			continue
		}
		total += len(t) + 1
		tags = append(tags, t)
	}
	tok, err := s.accessToken(r.Context(), uid)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var cur struct {
		Items []struct {
			Snippet struct {
				CategoryID      string `json:"categoryId"`
				DefaultLanguage string `json:"defaultLanguage"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := gcall(ctx, tok, "GET", "https://www.googleapis.com/youtube/v3/videos?part=snippet&id="+in.VideoID, nil, &cur); err != nil || len(cur.Items) == 0 {
		http.Error(w, "Could not find that video on your connected channel.", 404)
		return
	}
	sn := map[string]any{"title": in.Title, "description": in.Description, "tags": tags, "categoryId": cur.Items[0].Snippet.CategoryID}
	if l := cur.Items[0].Snippet.DefaultLanguage; l != "" {
		sn["defaultLanguage"] = l
	}
	if err := gcall(ctx, tok, "PUT", "https://www.googleapis.com/youtube/v3/videos?part=snippet", map[string]any{"id": in.VideoID, "snippet": sn}, nil); err != nil {
		http.Error(w, "YouTube rejected the update (is this your video, and did you allow YouTube access?).", 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}
