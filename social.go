package main

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

var platMult = map[string]float64{"instagram": 1, "tiktok": 2, "youtube": 0.8, "x": 0.25, "linkedin": 0.8, "facebook": 0.25}

// recommended posts per week (minimum to stay visible)
var platFreq = map[string]float64{"instagram": 3, "tiktok": 5, "youtube": 1, "x": 10, "linkedin": 3, "facebook": 3}

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
	mult, ok := platMult[pl]
	if !ok {
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

	// Engagement rate with size- and platform-adjusted benchmarks
	eng := in.Likes + in.Comments + in.Shares
	er := eng / f * 100
	good, okT := 4.0, 2.0
	if f >= 1e5 {
		good, okT = 1.5, 0.5
	} else if f >= 1e4 {
		good, okT = 2.5, 1.0
	}
	good, okT = good*mult, okT*mult
	ec := chk(E, "Engagement rate per post", rate(er, good, okT), fmt.Sprintf("%.2f%%", er), "Open with a hook in the first line/2 seconds, ask one clear question, and reply to every comment in the first hour.", 5)
	add(ec)
	if in.Likes > 0 {
		cr := in.Comments / in.Likes * 100
		add(chk(E, "Comment-to-like ratio", rate(cr, 3, 1), fmt.Sprintf("%.1f%%", cr), "Ask questions and invite opinions so people talk instead of only tapping like.", 3))
	}
	if eng > 0 && in.Shares > 0 {
		sh := in.Shares / eng * 100
		add(chk(E, "Share rate (virality)", rate(sh, 10, 4), fmt.Sprintf("%.1f%% of engagement", sh), "Make content worth sending to a friend: useful checklists, relatable moments, strong opinions.", 3))
	}

	// Reach
	var reach Check
	if in.Views > 0 {
		rr := in.Views / f * 100
		reach = chk(G, "Reach rate (views / followers)", rate(rr, 25, 10), fmt.Sprintf("%.0f%%", rr), "Improve the first 3 seconds, use trending but relevant audio/topics, and post when your audience is online.", 4)
		add(reach)
		res := eng / in.Views * 100
		add(chk(G, "Engagement per view", rate(res, 5, 2), fmt.Sprintf("%.1f%%", res), "People watch but don't act. Add a clear call to action and a reason to comment or save.", 3))
	}
	// Growth
	var growth Check
	g := 0.0
	if in.PrevFollowers > 0 {
		g = (f - in.PrevFollowers) / in.PrevFollowers * 100
		growth = chk(G, "Follower growth (30 days)", rate(g, 3, 1), fmt.Sprintf("%+.1f%%", g), "Double down on your best-performing formats, collaborate with similar creators, and post consistently.", 5)
		add(growth)
	}
	if in.Following > 0 {
		ratio := f / in.Following
		add(chk(PR, "Followers-to-following ratio", rate(ratio, 2, 1), fmt.Sprintf("%.1f : 1", ratio), "Follow fewer accounts and focus on earning followers; a healthy ratio builds credibility.", 2))
	}

	// Consistency
	pw := in.Posts30 / 4.3
	min := platFreq[pl]
	freq := chk(K, "Posting frequency", rate(pw, min, min/2), fmt.Sprintf("%.1f posts/week (aim for %.0f+)", pw, min), "Batch-create content once a week and schedule it so you never skip.", 4)
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
	if n := float64(len(chunks)); n > 0 {
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
		add(chk(CT, "Posts with a question or call to action", rate(share, 40, 20), fmt.Sprintf("%.0f%% of %d posts", share, len(chunks)), "End most posts with one question or one clear next step.", 3))
		r.KPIs = append(r.KPIs, KPI{Label: "Avg caption length", Value: fmt.Sprintf("%.0f chars", total/n), Note: fmt.Sprintf("%d captions analysed", len(chunks))})
	}

	// KPIs
	k := []KPI{kpiFrom(ec, "Engagement rate", fmt.Sprintf("Good \u2265 %.1f%% for this size/platform", good)), kpiFrom(freq, "Posts per week", fmt.Sprintf("Aim for %.0f+", min))}
	if growth.Name != "" {
		k = append(k, kpiFrom(growth, "30-day growth", "Good \u2265 3%"))
	}
	if reach.Name != "" {
		k = append(k, kpiFrom(reach, "Reach rate", "Good \u2265 25%"))
	}
	r.KPIs = append(k, r.KPIs...)

	// Charts
	r.Charts = append(r.Charts, Chart{Title: "Engagement rate vs benchmark (%)", Type: "bar", Labels: []string{"You", "OK threshold", "Good threshold"}, Datasets: []Dataset{{Name: "%", Data: []float64{math.Round(er*100) / 100, okT, good}}}})
	if in.PrevFollowers > 0 {
		cur, tgt := f, f
		ch := Chart{Title: "90-day follower projection", Type: "line", Labels: []string{"Now", "+30d", "+60d", "+90d"}, Datasets: []Dataset{{Name: "Current pace"}, {Name: "Target pace (+3%/month)"}}}
		for i := 0; i < 4; i++ {
			ch.Datasets[0].Data = append(ch.Datasets[0].Data, math.Round(cur))
			ch.Datasets[1].Data = append(ch.Datasets[1].Data, math.Round(tgt))
			cur *= 1 + g/100
			tgt *= 1.03
		}
		r.Charts = append(r.Charts, ch)
	}
	r.Notes = append(r.Notes, "Benchmarks are approximate rules of thumb adjusted for platform and account size. Social networks don't offer free public data, so these numbers come from what you entered.")
	r.Meta = map[string]string{"platform": pl, "handle": handle, "bio": clip(bio, 300), "goal": clip(in.Goal, 150), "perWeek": fmt.Sprintf("%.1f", pw)}
	r.finish()
	if in.UseAI {
		r.AI = s.insights(ctx, r, fmt.Sprintf("Platform %s, followers %.0f. Goal: %s. Bio: %s. Recent captions: %s", pl, f, clip(in.Goal, 150), clip(bio, 200), clip(in.Captions, 500)))
		if r.AI == nil {
			r.Notes = append(r.Notes, "AI written insights were unavailable this time. The measurements above are still accurate.")
		}
	}
	return r, nil
}
