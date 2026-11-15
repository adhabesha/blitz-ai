package main

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

var platMult = map[string]float64{"instagram": 1, "tiktok": 2, "youtube": 0.8, "x": 0.25, "linkedin": 0.8, "facebook": 0.25, "pinterest": 0.3, "threads": 0.5, "reddit": 0.4, "bluesky": 0.5, "telegram": 0.3}

// average engagement rate per follower (typical account, 10k-100k size). TikTok/Instagram/Facebook/X come from
// Socialinsider's published averages; the rest are estimates and are labelled as such in the report.
var platAvgER = map[string]float64{"instagram": 0.48, "tiktok": 3.7, "facebook": 0.15, "x": 0.12, "youtube": 0.8, "linkedin": 1.0, "pinterest": 0.2, "threads": 0.8, "reddit": 0.5, "bluesky": 0.5, "telegram": 1.0}

// sizeFactor scales the benchmark: small accounts engage a larger share of a small audience, big ones a smaller share.
func sizeFactor(f float64) float64 {
	switch {
	case f < 1e3:
		return 2.0
	case f < 1e4:
		return 1.5
	case f < 1e5:
		return 1.0
	}
	return 0.6
}

func avgF(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	t := 0.0
	for _, x := range v {
		t += x
	}
	return t / float64(len(v))
}

// recommended posts per week (minimum to stay visible)
var platFreq = map[string]float64{"instagram": 3, "tiktok": 5, "youtube": 1, "x": 10, "linkedin": 3, "facebook": 3, "pinterest": 5, "threads": 5, "reddit": 3, "bluesky": 5, "telegram": 5}

var ctaWords = []string{"link", "comment", "follow", "subscribe", "join", "dm", "download", "shop", "book", "sign up", "save", "share", "tag", "click", "try", "learn"}

func hasAny(s string, words []string) bool {
	s = strings.ToLower(s)
	for _, w := range words {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func (s *server) evalSocial(ctx context.Context, in evalIn) (*Report, error) {
	pl := strings.ToLower(in.Platform)
	if _, ok := platMult[pl]; !ok {
		return nil, userErr("Choose a platform")
	}
	handle := strings.TrimPrefix(clip(in.Handle, 40), "@")
	f := in.Followers
	if handle == "" || f <= 0 {
		return nil, userErr("Enter the handle and follower count")
	}
	for _, v := range []float64{f, in.PrevFollowers, in.Following, in.Posts30, in.Likes, in.Comments, in.Shares, in.Views} {
		if v < 0 || v > 5e9 || math.IsNaN(v) {
			return nil, userErr("Some numbers look invalid")
		}
	}
	r := &Report{Target: pl + " @" + handle}
	add := func(c Check) { r.Checks = append(r.Checks, c) }
	const E, G, K, PR, CT = "Engagement", "Growth & reach", "Consistency", "Profile", "Content"

	// ---- Method (see the benchmark note at the bottom of the report) ----
	// 1. Typical post = MEDIAN of recent posts (one viral post must not lift the score; HypeAuditor/Rival IQ use recent posts, 12 is the usual window).
	// 2. TikTok is measured per VIEW (Rival IQ, HypeAuditor, Socialinsider); other networks per FOLLOWER.
	// 3. Benchmarks are real platform averages scaled by account size (small accounts engage higher, big ones lower).
	// 4. Ratios that are meaningless on a tiny audience (under 100 followers) are shown but not scored.
	// 5. Few posts or followers = low confidence, which pulls the final score toward the middle.
	nPosts := len(in.PostEng)
	eng := in.Likes + in.Comments + in.Shares
	views := in.Views
	if nPosts >= 3 {
		eng = median(in.PostEng)
	}
	if len(in.PostViews) >= 3 {
		views = median(in.PostViews)
	}
	smallAud := f < 100
	viewBased := pl == "tiktok" && views >= 1

	var ec Check
	var erNote string
	var erVal, erGood, erOk float64
	switch {
	case viewBased:
		er := eng / views * 100
		erVal, erGood, erOk = er, 4.5, 2
		ec = chkG(E, "Engagement rate per post", er, 4.5, 2, fmt.Sprintf("%.1f%% of views (typical post)", er), "Open with a hook in the first 2 seconds, ask one clear question, and reply to every comment in the first hour.", 5)
		erNote = "Good ≥ 4.5% of views (TikTok average is about 3.7%)"
	case smallAud:
		erNote = "Needs 100+ followers for a reliable benchmark"
	default:
		er := eng / f * 100
		avg := platAvgER[pl]
		if avg == 0 {
			avg = 0.8
		}
		avg *= sizeFactor(f)
		good, okT := avg*1.25, avg*0.5
		erVal, erGood, erOk = er, good, okT
		ec = chkG(E, "Engagement rate per post", er, good, okT, fmt.Sprintf("%.2f%% of followers (typical post)", er), "Open with a hook in the first line/2 seconds, ask one clear question, and reply to every comment in the first hour.", 5)
		erNote = fmt.Sprintf("Good ≥ %.2f%% for this platform and size", good)
	}
	if ec.Name != "" {
		add(ec)
	}
	if in.Likes >= 5 {
		cr := in.Comments / in.Likes * 100
		add(chkG(E, "Comment-to-like ratio", cr, 3, 1, fmt.Sprintf("%.1f%%", cr), "Ask questions and invite opinions so people talk instead of only tapping like.", 3))
	}
	if in.Likes+in.Comments+in.Shares >= 10 && in.Shares > 0 {
		sh := in.Shares / (in.Likes + in.Comments + in.Shares) * 100
		add(chkG(E, "Share rate (virality)", sh, 10, 4, fmt.Sprintf("%.1f%% of engagement", sh), "Make content worth sending to a friend: useful checklists, relatable moments, strong opinions.", 3))
	}

	// Reach
	var reach Check
	if views > 0 && !smallAud {
		rr := views / f * 100
		reach = chkG(G, "Reach rate (views / followers)", rr, 25, 10, ratioText(rr, "followers"), "Improve the first 3 seconds, use trending but relevant audio/topics, and post when your audience is online.", 4)
		add(reach)
	}
	if views >= 50 && !viewBased {
		res := eng / views * 100
		add(chkG(G, "Engagement per view", res, 5, 2, fmt.Sprintf("%.1f%%", res), "People watch but don't act. Add a clear call to action and a reason to comment or save.", 3))
	}
	// Growth (a percentage on a tiny base is noise, so it needs 100+ followers a month ago)
	var growth Check
	g := 0.0
	if in.PrevFollowers >= 100 {
		g = (f - in.PrevFollowers) / in.PrevFollowers * 100
		growth = chkG(G, "Follower growth (30 days)", g, 2, 0.5, fmt.Sprintf("%+.1f%%", g), "Double down on your best-performing formats, collaborate with similar creators, and post consistently.", 5)
		add(growth)
	} else if in.PrevFollowers > 0 {
		g = (f - in.PrevFollowers) / in.PrevFollowers * 100
		r.Notes = append(r.Notes, fmt.Sprintf("Follower growth shows %+.0f followers, not a percentage, because under 100 followers a percentage would be misleading.", f-in.PrevFollowers))
	}
	if in.Following > 0 {
		ratio := f / in.Following
		add(chkG(PR, "Followers-to-following ratio", ratio, 2, 1, fmt.Sprintf("%.1f : 1", ratio), "Follow fewer accounts and focus on earning followers; a healthy ratio builds credibility.", 2))
	}

	// Consistency
	pw := in.Posts30 / 4.3
	min := platFreq[pl]
	freq := chkG(K, "Posting frequency", pw, min, min/2, fmt.Sprintf("%.1f posts/week (aim for %.0f+)", pw, min), "Batch-create content once a week and schedule it so you never skip.", 4)
	add(freq)

	// Profile
	bio := strings.TrimSpace(in.Bio)
	bl := utf8.RuneCountInString(bio)
	bioOK := bl >= 40 && hasAny(bio, ctaWords) && (strings.Contains(strings.ToLower(bio), "http") || strings.Contains(strings.ToLower(bio), ".com") || strings.Contains(strings.ToLower(bio), "link"))
	add(chk(PR, "Bio quality", ifs(bioOK, "pass", ifs(bl >= 20, "warn", "fail")), fmt.Sprintf("%d characters", bl), "State who you help and the result, add one clear call to action and a single link.", 3))

	// Captions
	var chunks []string
	for _, c := range strings.Split(strings.ReplaceAll(in.Captions, "\r", ""), "\n\n") {
		if c = strings.TrimSpace(c); c != "" {
			chunks = append(chunks, c)
		}
	}
	if len(chunks) > 30 {
		chunks = chunks[:30]
	}
	if n := float64(len(chunks)); n >= 1 {
		tags, cta, total := 0.0, 0.0, 0.0
		for _, c := range chunks {
			tags += float64(strings.Count(c, "#"))
			total += float64(utf8.RuneCountInString(c))
			if strings.Contains(c, "?") || hasAny(c, ctaWords) {
				cta++
			}
		}
		avgTags := tags / n
		add(chk(CT, "Hashtag use", ifs(avgTags >= 1 && avgTags <= 10, "pass", "warn"), fmt.Sprintf("%.1f per post", avgTags), "Use 3-8 specific hashtags; avoid none and avoid stuffing.", 2))
		share := cta / n * 100
		if n >= 4 { // a percentage of 2-3 captions says nothing
			add(chkG(CT, "Posts with a question or call to action", share, 40, 20, fmt.Sprintf("%.0f%% of %d posts", share, len(chunks)), "End most posts with one question or one clear next step.", 3))
		}
		r.KPIs = append(r.KPIs, KPI{Label: "Avg caption length", Value: fmt.Sprintf("%.0f chars", total/n), Note: fmt.Sprintf("%d captions analysed", len(chunks))})
	}

	// KPIs
	var k []KPI
	if ec.Name != "" {
		k = append(k, kpiFrom(ec, "Engagement rate", erNote))
	} else {
		k = append(k, KPI{Label: "Engagement rate", Value: "\u2013", Note: erNote})
	}
	k = append(k, kpiFrom(freq, "Posts per week", fmt.Sprintf("Aim for %.0f+", min)))
	if growth.Name != "" {
		k = append(k, kpiFrom(growth, "30-day growth", "Good \u2265 2%"))
	} else if in.PrevFollowers > 0 {
		k = append(k, KPI{Label: "30-day growth", Value: fmt.Sprintf("%+.0f followers", f-in.PrevFollowers), Note: "Percent shown once you pass 100 followers"})
	}
	if reach.Name != "" {
		k = append(k, kpiFrom(reach, "Views vs followers", "Good \u2265 25% of followers"))
	}

	// Confidence: how much real data stands behind the score
	conf := 0.7 // numbers typed in by hand
	nSrc := "typed-in averages"
	if nPosts > 0 {
		conf = clampF(float64(nPosts)/12, 0.25, 1)
		nSrc = fmt.Sprintf("%d recent posts", nPosts)
	}
	if f < 100 {
		conf *= 0.6
	} else if f < 1000 {
		conf *= 0.85
	}
	if views > 0 && views < 50 {
		conf *= 0.85
	}
	k = append(k, KPI{Label: "Data confidence", Value: fmt.Sprintf("%s \u00b7 %s", confLabel(conf), nSrc), Note: ifs(conf >= 0.75, "Enough data for a dependable score", "Score is pulled toward the middle until there is more data")})
	if conf < 0.75 {
		r.Notes = append(r.Notes, fmt.Sprintf("Confidence is %s: the score rests on %s and %s followers. With so little data a few posts swing the numbers, so the score is held closer to the middle (50) instead of showing an exaggerated high or low. It sharpens as you post more.", strings.ToLower(confLabel(conf)), nSrc, commas(f)))
	}
	if nPosts >= 5 {
		if mean := avgF(in.PostEng); eng > 0 && mean > 2.5*eng {
			r.Notes = append(r.Notes, fmt.Sprintf("One or two standout posts lift your average (%.0f) well above a typical post (%.0f). The report scores the typical post.", mean, eng))
		}
	}
	r.KPIs = append(k, r.KPIs...)

	// Charts
	if ec.Name != "" {
		r.Charts = append(r.Charts, Chart{Title: "Engagement rate vs benchmark (%)", Type: "bar", Labels: []string{"You (typical post)", "OK threshold", "Good threshold"}, Datasets: []Dataset{{Name: "%", Data: []float64{math.Round(erVal*100) / 100, math.Round(erOk*100) / 100, math.Round(erGood*100) / 100}}}})
	}
	if in.PrevFollowers >= 100 {
		cur, tgt := f, f
		ch := Chart{Title: "90-day follower projection", Type: "line", Labels: []string{"Now", "+30d", "+60d", "+90d"}, Datasets: []Dataset{{Name: "Current pace"}, {Name: "Target pace (+2%/month)"}}}
		for i := 0; i < 4; i++ {
			ch.Datasets[0].Data = append(ch.Datasets[0].Data, math.Round(cur))
			ch.Datasets[1].Data = append(ch.Datasets[1].Data, math.Round(tgt))
			cur *= 1 + g/100
			tgt *= 1.02
		}
		r.Charts = append(r.Charts, ch)
	}
	r.Notes = append(r.Notes, "How this is scored: the typical (median) recent post, not the average; TikTok per view and other networks per follower, the way Rival IQ, Socialinsider and HypeAuditor measure it; benchmarks start from published 2025-26 platform averages (Socialinsider: TikTok 3.7% per view, Instagram 0.48%, Facebook 0.15%, X 0.12% per follower) and scale with account size; other networks use estimated averages. Scores move smoothly instead of in jumps, and are held nearer the middle when there is little data.")
	r.Meta = map[string]string{"platform": pl, "handle": handle, "bio": clip(bio, 300), "goal": clip(in.Goal, 150), "perWeek": fmt.Sprintf("%.1f", pw), "confidence": fmt.Sprintf("%.0f", conf*100)}
	r.finishWith(conf)
	if in.UseAI {
		r.AI = s.insights(ctx, r, fmt.Sprintf("Platform %s, followers %.0f. Goal: %s. Bio: %s. Recent captions: %s", pl, f, clip(in.Goal, 150), clip(bio, 200), clip(in.Captions, 500)))
		if r.AI == nil {
			r.Notes = append(r.Notes, "AI written insights were unavailable this time. The measurements above are still accurate.")
		}
	}
	return r, nil
}
