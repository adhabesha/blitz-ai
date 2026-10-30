package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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
	"youtube":   "Titles under 70 characters, main keyword first, with a curiosity or benefit hook. The first 150 characters of the description must contain the main keyword and a hook. Description 800-1500 characters with a call to action and placeholders for links and timestamps. 10-15 tags mixing broad and long-tail keywords, under 450 characters total. 3 hashtags.",
	"instagram": "Caption first line is a hook under 125 characters, then value, then one call to action. 3-8 relevant hashtags mixing niche and mid-size. Under 2200 characters.",
	"tiktok":    "Caption under 150 characters with a hook and 3-5 hashtags. Hooks must work in the first 3 seconds of video and on-screen text.",
	"x":         "Posts under 260 characters, one idea, strong first words, at most 2 hashtags, an invitation to reply.",
	"linkedin":  "A hook in the first line (before the 'see more' cut), short paragraphs, a concrete lesson or story, a question at the end, 3 hashtags.",
	"facebook":  "A conversational opener, 1-3 short paragraphs, a clear call to action, at most 3 hashtags.",
	"app":       "App store listing: title under 30 characters (brand + one keyword), a 30-character subtitle, a 100-character keyword list, and a description whose first 3 lines sell the main benefit, then features, social proof and a call to action.",
	"bio":       "Give 5 bio options under 150 characters: who you help, the result they get, one call to action and a prompt to use the link. No emojis spam, no invented facts.",
	"ideas":     "Give 8 specific content ideas for the next 4 weeks. Each idea is a ready-to-use title or hook. Mix educational, story, opinion and proof content, and build on the topic that already works.",
	"howto":     "Turn the task into numbered, concrete implementation steps a beginner can follow today with free tools. Add 3 ready-to-use examples or templates.",
	"faq":       "Write 5 questions real buyers ask about this business and a concise, factual 2-sentence answer for each, in the same order. Do not invent prices, awards or statistics.",
	"website":   "SEO title under 60 characters with the main keyword first, meta description 140-155 characters with a benefit and call to action, plus one H1 suggestion in the notes.",
}

type optIn struct {
	Platform    string   `json:"platform"`
	VideoID     string   `json:"videoId"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Topic       string   `json:"topic"`
	Audience    string   `json:"audience"`
	Goal        string   `json:"goal"`
}

type optOut struct {
	Titles      []string `json:"titles"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Hashtags    []string `json:"hashtags"`
	Hooks       []string `json:"hooks"`
	Ideas       []string `json:"ideas"`
	Notes       []string `json:"notes"`
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
	prompt := fmt.Sprintf("You are an expert %s growth strategist. Optimise the content below for maximum reach and clicks using the platform's published best practices, and never invent facts about the creator.\nRules: %s\nCurrent title: %s\nCurrent description or caption: %s\nCurrent tags: %s\nTopic: %s\nAudience: %s\nGoal: %s\nReturn JSON only with keys: titles (%d distinct options; for non-video platforms caption openers; for ideas the content ideas; for faq the questions; for bio the bio options), description (the final improved full description, caption or bio, ready to paste; for howto the numbered steps; for faq a one-line summary), tags (keywords without #; empty list if the platform doesn't use tags), hashtags (with #), hooks (3 strong opening lines; for faq the matching answers in the same order as titles; for howto 3 ready-to-use examples), ideas (3 thumbnail or visual ideas), notes (3 short A/B testing tips).",
		pl, rules, clip(title, 150), clip(desc, 1500), clip(strings.Join(tags, ", "), 400), clip(in.Topic, 400), clip(in.Audience, 150), clip(in.Goal, 150), n)
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
	json.NewEncoder(w).Encode(map[string]any{"result": out, "current": map[string]any{"title": title, "description": desc, "tags": tags}})
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
