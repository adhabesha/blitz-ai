package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	reYTID    = regexp.MustCompile(`^UC[\w-]{22}$`)
	reChanURL = regexp.MustCompile(`youtube\.com/channel/(UC[\w-]{22})`)
	reHandle  = regexp.MustCompile(`youtube\.com/(@[^/?#\s]+)`)
	reLegacy  = regexp.MustCompile(`youtube\.com/(?:c|user)/([^/?#\s]+)`)
	reDur     = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?$`)
)

func ytGet(ctx context.Context, path string, q url.Values, out any) error {
	q.Set("key", os.Getenv("YOUTUBE_API_KEY"))
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/youtube/v3/"+path+"?"+q.Encode(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		return userErr("YouTube refused the request: the free daily quota may be used up, or YOUTUBE_API_KEY is wrong/not enabled for the YouTube Data API.")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("youtube status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func parseYT(in string) (id, handle, name string) {
	in = strings.TrimSpace(in)
	if m := reChanURL.FindStringSubmatch(in); m != nil {
		return m[1], "", ""
	}
	if m := reHandle.FindStringSubmatch(in); m != nil {
		return "", m[1], ""
	}
	if m := reLegacy.FindStringSubmatch(in); m != nil {
		return "", "", m[1]
	}
	if reYTID.MatchString(in) {
		return in, "", ""
	}
	if strings.HasPrefix(in, "@") {
		return "", in, ""
	}
	return "", "", in
}

func parseDur(s string) float64 {
	m := reDur.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	return atof(m[1])*3600 + atof(m[2])*60 + atof(m[3])
}

type ytChannelResp struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title       string `json:"title"`
			PublishedAt string `json:"publishedAt"`
		} `json:"snippet"`
		Statistics struct {
			ViewCount  string `json:"viewCount"`
			Subs       string `json:"subscriberCount"`
			VideoCount string `json:"videoCount"`
			Hidden     bool   `json:"hiddenSubscriberCount"`
		} `json:"statistics"`
		ContentDetails struct {
			Related struct {
				Uploads string `json:"uploads"`
			} `json:"relatedPlaylists"`
		} `json:"contentDetails"`
	} `json:"items"`
}

type ytVideosResp struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			PublishedAt string   `json:"publishedAt"`
			Tags        []string `json:"tags"`
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

type ytVid struct {
	ID, Title                string
	Pub                      time.Time
	Views, Likes, Comments   float64
	Dur                      float64
	Tags, DescLen            int
	PerDay, Eng              float64
}

func (s *server) evalYouTube(ctx context.Context, in evalIn) (*Report, error) {
	if os.Getenv("YOUTUBE_API_KEY") == "" {
		return nil, userErr("YouTube lookup isn't set up yet: add YOUTUBE_API_KEY in Render > Environment (see the setup steps).")
	}
	q := clip(in.Query, 200)
	if q == "" {
		return nil, userErr("Paste a YouTube channel link or @handle")
	}
	chID, handle, name := parseYT(q)
	vals := url.Values{"part": {"snippet,statistics,contentDetails"}}
	switch {
	case chID != "":
		vals.Set("id", chID)
	case handle != "":
		vals.Set("forHandle", handle)
	default:
		var sr struct {
			Items []struct {
				ID struct {
					ChannelID string `json:"channelId"`
				} `json:"id"`
			} `json:"items"`
		}
		if err := ytGet(ctx, "search", url.Values{"part": {"snippet"}, "type": {"channel"}, "maxResults": {"1"}, "q": {name}}, &sr); err != nil {
			return nil, err
		}
		if len(sr.Items) == 0 {
			return nil, userErr("Channel not found. Paste the full channel link or @handle.")
		}
		vals.Set("id", sr.Items[0].ID.ChannelID)
	}
	var ch ytChannelResp
	if err := ytGet(ctx, "channels", vals, &ch); err != nil {
		return nil, err
	}
	if len(ch.Items) == 0 {
		return nil, userErr("Channel not found. Paste the full channel link or @handle.")
	}
	c := ch.Items[0]
	var pl struct {
		Items []struct {
			ContentDetails struct {
				VideoID string `json:"videoId"`
			} `json:"contentDetails"`
		} `json:"items"`
	}
	if err := ytGet(ctx, "playlistItems", url.Values{"part": {"contentDetails"}, "playlistId": {c.ContentDetails.Related.Uploads}, "maxResults": {"30"}}, &pl); err != nil {
		return nil, err
	}
	var ids []string
	for _, it := range pl.Items {
		ids = append(ids, it.ContentDetails.VideoID)
	}
	if len(ids) == 0 {
		return nil, userErr("This channel has no public videos to analyse.")
	}
	var vr ytVideosResp
	if err := ytGet(ctx, "videos", url.Values{"part": {"snippet,statistics,contentDetails"}, "id": {strings.Join(ids, ",")}}, &vr); err != nil {
		return nil, err
	}
	now := time.Now()
	var vids []ytVid
	for _, it := range vr.Items {
		t, _ := time.Parse(time.RFC3339, it.Snippet.PublishedAt)
		v := ytVid{ID: it.ID, Title: it.Snippet.Title, Pub: t, Views: atof(it.Statistics.Views), Likes: atof(it.Statistics.Likes), Comments: atof(it.Statistics.Comments), Dur: parseDur(it.ContentDetails.Duration), Tags: len(it.Snippet.Tags), DescLen: utf8.RuneCountInString(it.Snippet.Description)}
		age := math.Max(now.Sub(t).Hours()/24, 1)
		v.PerDay = v.Views / age
		if v.Views > 0 {
			v.Eng = (v.Likes + v.Comments) / v.Views * 100
		}
		vids = append(vids, v)
	}
	sort.Slice(vids, func(i, j int) bool { return vids[i].Pub.After(vids[j].Pub) })
	n := float64(len(vids))

	subs := atof(c.Statistics.Subs)
	var views, engs []float64
	var sumEng, n30, shorts, withNum, withDesc, withTags, titleLen float64
	wdSum, wdCnt := make([]float64, 7), make([]float64, 7)
	for _, v := range vids {
		views = append(views, v.Views)
		engs = append(engs, v.Eng)
		sumEng += v.Eng
		if now.Sub(v.Pub).Hours() <= 24*30 {
			n30++
		}
		if v.Dur > 0 && v.Dur <= 180 {
			shorts++
		}
		if strings.IndexFunc(v.Title, unicode.IsDigit) >= 0 {
			withNum++
		}
		if v.DescLen >= 200 {
			withDesc++
		}
		if v.Tags >= 5 {
			withTags++
		}
		titleLen += float64(utf8.RuneCountInString(v.Title))
		wd := int(v.Pub.Weekday())
		wdSum[wd] += v.Views
		wdCnt[wd]++
	}
	med := median(views)
	avgViews := 0.0
	for _, x := range views {
		avgViews += x
	}
	avgViews /= n
	maxViews := 0.0
	for _, x := range views {
		maxViews = math.Max(maxViews, x)
	}
	avgEng := sumEng / n
	perWeek := n30 / 4.3
	daysSince := now.Sub(vids[0].Pub).Hours() / 24
	reach := 0.0
	if subs > 0 {
		reach = med / subs * 100
	}
	hit := 0.0
	if med > 0 {
		hit = maxViews / med
	}
	avgTitle := titleLen / n

	r := &Report{Target: c.Snippet.Title}
	add := func(x Check) { r.Checks = append(r.Checks, x) }
	const A, E, K, O = "Audience & reach", "Engagement", "Consistency", "Optimization"
	reachC := chk(A, "Typical views vs subscribers", rate(reach, 15, 5), fmt.Sprintf("%.0f%% (median video)", reach), "Your videos reach too few of your subscribers. Improve thumbnails and titles, and post on your audience's active days.", 5)
	if subs > 0 && !c.Statistics.Hidden {
		add(reachC)
	}
	engC := chk(E, "Engagement rate (likes + comments / views)", rate(avgEng, 4, 2), fmt.Sprintf("%.2f%%", avgEng), "Ask one clear question per video, pin a comment, and reply to comments in the first hour.", 5)
	add(engC)
	likeRate := 0.0
	var totalLikes, totalViews float64
	for _, v := range vids {
		totalLikes += v.Likes
		totalViews += v.Views
	}
	if totalViews > 0 {
		likeRate = totalLikes / totalViews * 100
	}
	add(chk(E, "Like rate", rate(likeRate, 3, 1.5), fmt.Sprintf("%.2f%%", likeRate), "Remind viewers to like at the moment you deliver value, not at the start.", 3))
	cadC := chk(K, "Upload frequency (last 30 days)", rate(perWeek, 1, 0.5), fmt.Sprintf("%.1f videos/week", perWeek), "Aim for at least one video per week; batch-record to stay consistent.", 5)
	add(cadC)
	add(chk(K, "Days since last upload", low(daysSince, 7, 21), fmt.Sprintf("%.0f days", daysSince), "Long gaps lower recommendations. Publish something soon, even a short.", 4))
	add(chk(A, "Hit potential (best video / median)", rate(hit, 3, 1.8), fmt.Sprintf("%.1fx", hit), "Study your best video's topic, title and thumbnail, then make variations of it.", 3))
	add(chk(O, "Title length", ifs(avgTitle >= 35 && avgTitle <= 70, "pass", "warn"), fmt.Sprintf("%.0f characters on average", avgTitle), "Keep titles at 40-70 characters with the main keyword first.", 3))
	add(chk(O, "Descriptions \u2265 200 characters", rate(withDesc/n*100, 70, 40), fmt.Sprintf("%.0f%% of videos", withDesc/n*100), "Write 200+ character descriptions with keywords, links and a call to action.", 3))
	add(chk(O, "Videos with 5+ tags", rate(withTags/n*100, 70, 40), fmt.Sprintf("%.0f%% of videos", withTags/n*100), "Add 8-15 relevant tags mixing broad and long-tail keywords.", 2))
	add(chk(O, "Titles with numbers", rate(withNum/n*100, 30, 15), fmt.Sprintf("%.0f%% of videos", withNum/n*100), "Numbers and specifics (e.g. '5 mistakes') often lift click-through.", 1))

	subsLabel := human(subs)
	if c.Statistics.Hidden {
		subsLabel = "hidden"
	}
	r.KPIs = []KPI{
		{Label: "Subscribers", Value: subsLabel, Note: human(atof(c.Statistics.VideoCount)) + " videos"},
		{Label: "Total views", Value: human(atof(c.Statistics.ViewCount)), Note: "lifetime"},
		{Label: "Median video views", Value: human(med), Note: fmt.Sprintf("avg %s over last %d", human(avgViews), len(vids))},
		kpiFrom(engC, "Engagement rate", "Good \u2265 4%"),
		kpiFrom(cadC, "Uploads per week", "Aim for 1+"),
		{Label: "Short-form share", Value: fmt.Sprintf("%.0f%%", shorts/n*100), Note: "videos \u2264 3 min"},
	}
	if subs > 0 && !c.Statistics.Hidden {
		r.KPIs = append(r.KPIs, kpiFrom(reachC, "Reach vs subscribers", "Good \u2265 15%"))
	}

	// Charts (oldest to newest)
	vc := Chart{Title: "Views per recent video (oldest \u2192 newest)", Type: "bar", Datasets: []Dataset{{Name: "Views"}}}
	ec := Chart{Title: "Engagement rate per video (%)", Type: "line", Datasets: []Dataset{{Name: "Engagement %"}}}
	for i := len(vids) - 1; i >= 0; i-- {
		vc.Labels = append(vc.Labels, trunc(vids[i].Title, 16))
		vc.Datasets[0].Data = append(vc.Datasets[0].Data, vids[i].Views)
		ec.Labels = append(ec.Labels, trunc(vids[i].Title, 16))
		ec.Datasets[0].Data = append(ec.Datasets[0].Data, math.Round(vids[i].Eng*100)/100)
	}
	days := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	wc := Chart{Title: "Average views by upload weekday (UTC, small sample)", Type: "bar", Labels: days, Datasets: []Dataset{{Name: "Avg views"}}}
	bestDay, bestAvg := -1, 0.0
	for i := 0; i < 7; i++ {
		a := 0.0
		if wdCnt[i] > 0 {
			a = wdSum[i] / wdCnt[i]
		}
		wc.Datasets[0].Data = append(wc.Datasets[0].Data, math.Round(a))
		if a > bestAvg && wdCnt[i] >= 2 {
			bestDay, bestAvg = i, a
		}
	}
	target := math.Max(2, perWeek*1.3)
	pc := Chart{Title: "Projected cumulative views (next 3 months)", Type: "line", Labels: []string{"Now", "Month 1", "Month 2", "Month 3"}, Datasets: []Dataset{{Name: fmt.Sprintf("Current pace (%.1f/week)", perWeek)}, {Name: fmt.Sprintf("Recommended pace (%.1f/week)", target)}}}
	for m := 0; m <= 3; m++ {
		pc.Datasets[0].Data = append(pc.Datasets[0].Data, math.Round(med*perWeek*4.3*float64(m)))
		pc.Datasets[1].Data = append(pc.Datasets[1].Data, math.Round(med*target*4.3*float64(m)))
	}
	r.Charts = []Chart{vc, ec, wc, pc}

	// Tables
	row := func(v ytVid) Row {
		return Row{Cells: []string{trunc(v.Title, 55), human(v.Views), fmt.Sprintf("%.1f", v.PerDay), fmt.Sprintf("%.2f%%", v.Eng), v.Pub.Format("2006-01-02")}}
	}
	byViews := append([]ytVid(nil), vids...)
	sort.Slice(byViews, func(i, j int) bool { return byViews[i].Views > byViews[j].Views })
	cols := []string{"Video", "Views", "Views/day", "Engagement", "Published"}
	top := Table{Title: "Top performing videos", Columns: cols}
	for i := 0; i < len(byViews) && i < 5; i++ {
		top.Rows = append(top.Rows, row(byViews[i]))
	}
	var mature []ytVid
	for _, v := range vids {
		if now.Sub(v.Pub).Hours() >= 24*7 {
			mature = append(mature, v)
		}
	}
	sort.Slice(mature, func(i, j int) bool { return mature[i].PerDay < mature[j].PerDay })
	weak := Table{Title: "Weakest videos (views per day, older than 7 days) \u2013 optimize these", Columns: cols}
	for i := 0; i < len(mature) && i < 5; i++ {
		weak.Rows = append(weak.Rows, row(mature[i]))
	}
	r.Tables = []Table{top, weak}

	// Prioritized plan (some items can be automated)
	if perWeek < 1 {
		r.Plan = append(r.Plan, PlanItem{Title: "Publish at least one video every week", Why: fmt.Sprintf("You published %.1f per week recently. Projection: reaching %.1f/week could add roughly %s views over 3 months.", perWeek, target, human(math.Max(0, (target-perWeek)*med*4.3*3))), Impact: 5, Effort: 3})
	}
	if len(mature) > 0 {
		w := mature[0]
		r.Plan = append(r.Plan, PlanItem{Title: "Rewrite title, description and tags of \"" + trunc(w.Title, 40) + "\"", Why: "It earns the fewest views per day. Better metadata is the fastest low-effort fix.", Impact: 4, Effort: 1, Auto: "optimize-video:" + w.ID})
	}
	if len(byViews) > 0 {
		t := byViews[0]
		r.Plan = append(r.Plan, PlanItem{Title: "Make 3 more videos like \"" + trunc(t.Title, 40) + "\"", Why: fmt.Sprintf("It is %.1fx your median video. Repeat the topic, hook and thumbnail style.", hit), Impact: 5, Effort: 3, Auto: "optimize-video:" + t.ID})
	}
	if withDesc/n*100 < 70 || withTags/n*100 < 70 {
		r.Plan = append(r.Plan, PlanItem{Title: "Add full descriptions (200+ chars) and 8-15 tags to all videos", Why: fmt.Sprintf("Only %.0f%% have long descriptions and %.0f%% have 5+ tags.", withDesc/n*100, withTags/n*100), Impact: 3, Effort: 2})
	}
	if avgEng < 4 {
		r.Plan = append(r.Plan, PlanItem{Title: "Add a question and a pinned comment to every video", Why: fmt.Sprintf("Engagement is %.2f%% (good \u2265 4%%).", avgEng), Impact: 4, Effort: 1})
	}
	if shorts/n < 0.2 {
		r.Plan = append(r.Plan, PlanItem{Title: "Test short-form videos (\u2264 3 min) to reach new viewers", Why: fmt.Sprintf("Only %.0f%% of your recent uploads are short-form.", shorts/n*100), Impact: 3, Effort: 2})
	}
	if bestDay >= 0 {
		r.Plan = append(r.Plan, PlanItem{Title: "Schedule uploads for " + days[bestDay], Why: fmt.Sprintf("Your %s uploads average %s views, your best day so far (small sample).", days[bestDay], human(bestAvg)), Impact: 2, Effort: 1})
	}
	r.Notes = append(r.Notes, "Based on public data for the last "+strconv.Itoa(len(vids))+" videos. Projections assume your median video performance continues; they are estimates, not guarantees. Connect Google to unlock watch time, retention and subscriber growth for your own channel.")
	r.Meta = map[string]string{"channel": c.Snippet.Title, "perWeek": fmt.Sprintf("%.1f", perWeek), "niche": clip(titlesSample(vids), 300)}
	if len(mature) > 0 {
		r.Meta["weakId"] = mature[0].ID
		r.Meta["weakTitle"] = clip(mature[0].Title, 100)
	}
	if len(byViews) > 0 {
		r.Meta["topId"] = byViews[0].ID
		r.Meta["topTitle"] = clip(byViews[0].Title, 100)
	}
	if bestDay >= 0 {
		r.Meta["bestDay"] = days[bestDay]
	}
	r.finish()
	if in.UseAI {
		r.AI = s.insights(ctx, r, "YouTube channel "+c.Snippet.Title+". Topics seen in titles: "+titlesSample(vids))
		if r.AI == nil {
			r.Notes = append(r.Notes, "AI written insights were unavailable this time. The measurements above are still accurate.")
		}
	}
	return r, nil
}

func titlesSample(v []ytVid) string {
	var t []string
	for i := 0; i < len(v) && i < 8; i++ {
		t = append(t, clip(v[i].Title, 60))
	}
	return strings.Join(t, " | ")
}
