package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type itunesApp struct {
	TrackName      string   `json:"trackName"`
	SellerName     string   `json:"sellerName"`
	AvgRating      float64  `json:"averageUserRating"`
	RatingCount    float64  `json:"userRatingCount"`
	AvgRatingCur   float64  `json:"averageUserRatingForCurrentVersion"`
	RatingCountCur float64  `json:"userRatingCountForCurrentVersion"`
	Updated        string   `json:"currentVersionReleaseDate"`
	Description    string   `json:"description"`
	Screens        []string `json:"screenshotUrls"`
	Price          float64  `json:"price"`
	Langs          []string `json:"languageCodesISO2A"`
	Genre          string   `json:"primaryGenreName"`
}

var (
	reAppID   = regexp.MustCompile(`id(\d{6,})`)
	reAppCtry = regexp.MustCompile(`apps\.apple\.com/([a-z]{2})/`)
)

func itunesLookup(ctx context.Context, q string) (*itunesApp, error) {
	var u string
	if m := reAppID.FindStringSubmatch(q); m != nil {
		country := "us"
		if c := reAppCtry.FindStringSubmatch(q); c != nil {
			country = c[1]
		}
		u = "https://itunes.apple.com/lookup?id=" + m[1] + "&country=" + country
	} else {
		u = "https://itunes.apple.com/search?entity=software&limit=1&term=" + url.QueryEscape(q)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var d struct {
		Results []itunesApp `json:"results"`
	}
	if json.NewDecoder(resp.Body).Decode(&d) != nil || len(d.Results) == 0 {
		return nil, userErr("App not found on the App Store. Try the full App Store link.")
	}
	return &d.Results[0], nil
}

func (s *server) evalApp(ctx context.Context, in evalIn) (*Report, error) {
	const R, F, L = "Reputation", "Freshness", "Store listing"
	var name string
	var rating, ratings, days, descLen, shots, langs, curRating, curCount, titleLen float64
	r := &Report{}
	ios := in.Store != "android"
	if ios {
		q := clip(in.Query, 200)
		if q == "" {
			return nil, userErr("Paste the App Store link or the app name")
		}
		a, err := itunesLookup(ctx, q)
		if err != nil {
			if _, ok := err.(userErr); ok {
				return nil, err
			}
			return nil, userErr("Could not reach the App Store. Try again.")
		}
		name = a.TrackName
		rating, ratings = a.AvgRating, a.RatingCount
		curRating, curCount = a.AvgRatingCur, a.RatingCountCur
		if t, err := time.Parse(time.RFC3339, a.Updated); err == nil {
			days = time.Since(t).Hours() / 24
		}
		descLen = float64(utf8.RuneCountInString(a.Description))
		shots = float64(len(a.Screens))
		langs = float64(len(a.Langs))
		titleLen = float64(utf8.RuneCountInString(a.TrackName))
		r.KPIs = append(r.KPIs, KPI{Label: "Developer", Value: a.SellerName, Note: a.Genre})
	} else {
		name = clip(in.Title, 60)
		if name == "" {
			name = "Android app"
		}
		if in.Rating <= 0 || in.Rating > 5 {
			return nil, userErr("Enter the app's rating (0-5) and ratings count from its Google Play page")
		}
		rating, ratings, days, descLen, shots = in.Rating, in.Ratings, in.DaysSince, in.DescLen, in.Shots
		langs = -1
	}
	r.Target = name

	add := func(c Check) { r.Checks = append(r.Checks, c) }
	add(chk(R, "Average rating", rate(rating, 4.5, 4.0), fmt.Sprintf("%.2f / 5", rating), "Prompt happy users for a rating after a success moment, answer negative reviews, and fix the top complaints.", 5))
	add(chk(R, "Number of ratings", rate(ratings, 1000, 100), strconv.FormatFloat(ratings, 'f', 0, 64), "Add an in-app rating prompt (after the 3rd successful use) to build review volume.", 4))
	if ios && curCount >= 10 {
		add(chk(R, "Current-version rating", rate(curRating, rating-0.2, rating-0.5), fmt.Sprintf("%.2f from %.0f ratings", curRating, curCount), "The latest release is rated lower than your overall rating. Check recent reviews for regressions.", 3))
	}
	add(chk(F, "Days since last update", low(days, 30, 90), fmt.Sprintf("%.0f days", days), "Ship updates at least monthly; stores and users reward actively maintained apps.", 4))
	add(chk(L, "Screenshots", rate(shots, 5, 3), fmt.Sprintf("%.0f", shots), "Use 5-8 screenshots; lead with the main benefit and add short captions.", 4))
	add(chk(L, "Description length", rate(descLen, 1200, 500), fmt.Sprintf("%.0f characters", descLen), "Write 1,200+ characters: a strong first 2 lines, key features, social proof, and keywords.", 3))
	if ios {
		add(chk(L, "Title length", low(titleLen, 30, 40), fmt.Sprintf("%.0f characters", titleLen), "App Store titles are limited to 30 characters. Put the brand first and one keyword after.", 2))
		add(chk(L, "Localization", rate(langs, 5, 2), fmt.Sprintf("%.0f languages", langs), "Translate the listing and app into your top markets to increase downloads.", 2))
	}

	r.KPIs = append(r.KPIs,
		KPI{Label: "Rating", Value: fmt.Sprintf("%.2f", rating), Note: "Target 4.5+", Status: rate(rating, 4.5, 4.0)},
		KPI{Label: "Ratings", Value: strconv.FormatFloat(ratings, 'f', 0, 64), Note: "Target 1,000+", Status: rate(ratings, 1000, 100)},
		KPI{Label: "Last update", Value: fmt.Sprintf("%.0f days ago", days), Note: "Target \u2264 30 days", Status: low(days, 30, 90)},
		KPI{Label: "Screenshots", Value: fmt.Sprintf("%.0f", shots), Note: "Target 5+", Status: rate(shots, 5, 3)},
	)
	r.Charts = append(r.Charts, Chart{Title: "Rating vs target", Type: "bar", Labels: []string{"Your rating", "Target"}, Datasets: []Dataset{{Name: "Stars", Data: []float64{rating, 4.5}}}})
	if !ios {
		r.Notes = append(r.Notes, "Android numbers come from what you entered; Google Play has no free public API, so keep them current.")
	}
	r.finish()
	if in.UseAI {
		r.AI = s.insights(ctx, r, "Mobile app store optimization (ASO) for "+name+". Suggest listing, keyword and review-growth improvements.")
		if r.AI == nil {
			r.Notes = append(r.Notes, "AI written insights were unavailable this time. The measurements above are still accurate.")
		}
	}
	_ = strings.TrimSpace
	return r, nil
}
