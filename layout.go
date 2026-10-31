package main

import "strings"

type pageRule struct{ Sub, Page string }

type kindLayout struct {
	Pages                   []PageDef
	Charts, Tables, KPIs    []pageRule
	Cats                    map[string]string
	DefChart, DefTable, Itm string
}

func hasSub(s, sub string) bool { return strings.Contains(strings.ToLower(s), strings.ToLower(sub)) }

func route(rules []pageRule, text, def string) string {
	for _, r := range rules {
		if hasSub(text, r.Sub) {
			return r.Page
		}
	}
	return def
}

var layouts = map[string]kindLayout{
	"youtube": {
		Pages:  []PageDef{{"overview", "Overview"}, {"shortlong", "Short vs Long"}, {"videos", "Videos"}, {"tags", "Viral tags"}, {"time", "Time"}, {"fixes", "Fixes & plan"}},
		Charts: []pageRule{{"short vs long", "shortlong"}, {"views share", "shortlong"}, {"weekday", "time"}, {"uploads per week", "time"}, {"projected", "time"}},
		Tables: []pageRule{{"short vs long", "shortlong"}, {"tags", "tags"}, {"words in", "tags"}, {"weakest", "videos"}},
		DefChart: "overview", DefTable: "overview", Itm: "videos",
	},
	"website": {
		Pages:  []PageDef{{"overview", "Overview"}, {"seo", "SEO"}, {"technical", "Technical & speed"}, {"ai", "AI visibility"}, {"fixes", "Fixes & plan"}},
		Charts: []pageRule{{"brands ai", "ai"}},
		Cats:   map[string]string{"On-page SEO": "seo", "Technical": "technical", "Crawl & AI readiness": "technical", "Performance": "technical", "Accessibility": "technical", "AI visibility": "ai"},
		DefChart: "overview", DefTable: "overview",
	},
	"google": {
		Pages:  []PageDef{{"overview", "Overview"}, {"search", "Search"}, {"traffic", "Traffic"}, {"youtube", "YouTube"}, {"fixes", "Fixes & plan"}},
		Charts: []pageRule{{"search", "search"}, {"impressions", "search"}, {"sessions", "traffic"}, {"channel", "traffic"}},
		Tables: []pageRule{{"search", "search"}, {"striking", "search"}, {"high-ranking", "search"}, {"top pages", "traffic"}},
		KPIs:   []pageRule{{"search", "search"}, {"ctr", "search"}, {"position", "search"}, {"clicks", "search"}, {"sessions", "traffic"}, {"users", "traffic"}, {"engagement rate", "traffic"}, {"key events", "traffic"}},
		Cats:   map[string]string{"Search (Google)": "search", "Traffic (GA4)": "traffic"},
		DefChart: "overview", DefTable: "overview", Itm: "youtube",
	},
	"social": {
		Pages:  []PageDef{{"overview", "Overview"}, {"posts", "Posts"}, {"growth", "Growth"}, {"fixes", "Fixes & plan"}},
		Charts: []pageRule{{"per recent post", "posts"}, {"views / reach", "posts"}, {"weekday", "posts"}, {"benchmark", "growth"}, {"projection", "growth"}},
		Tables: []pageRule{{"posts", "posts"}},
		Cats:   map[string]string{"Engagement": "posts", "Content": "posts", "Growth & reach": "growth", "Consistency": "growth"},
		DefChart: "overview", DefTable: "overview", Itm: "posts",
	},
	"app": {
		Pages:  []PageDef{{"overview", "Overview"}, {"reputation", "Reputation"}, {"listing", "Store listing"}, {"fixes", "Fixes & plan"}},
		Charts: []pageRule{{"rating", "reputation"}},
		Cats:   map[string]string{"Reputation": "reputation", "Freshness": "listing", "Store listing": "listing"},
		DefChart: "overview", DefTable: "overview", Itm: "listing",
	},
}

// layoutReport assigns every chart, table, KPI, check and item to a page (a left-side tab).
func layoutReport(r *Report) {
	lk := r.Kind
	if lk == "social_connected" {
		lk = "social"
	}
	l, ok := layouts[lk]
	if !ok {
		return
	}
	count := map[string]int{}
	for i := range r.Charts {
		if r.Charts[i].Page == "" {
			r.Charts[i].Page = route(l.Charts, r.Charts[i].Title, l.DefChart)
		}
		count[r.Charts[i].Page]++
	}
	for i := range r.Tables {
		if r.Tables[i].Page == "" {
			r.Tables[i].Page = route(l.Tables, r.Tables[i].Title, l.DefTable)
		}
		count[r.Tables[i].Page]++
	}
	for i := range r.KPIs {
		if r.KPIs[i].Page == "" {
			r.KPIs[i].Page = route(l.KPIs, r.KPIs[i].Label, "overview")
		}
		if r.KPIs[i].Page != "overview" {
			count[r.KPIs[i].Page]++
		}
	}
	for i := range r.Checks {
		c := &r.Checks[i]
		p, ok := l.Cats[c.Category]
		if !ok {
			p = "fixes"
			if lk == "google" {
				p = "youtube"
			}
		}
		if c.Page == "" {
			c.Page = p
		}
		count[c.Page]++
	}
	for i := range r.Items {
		if r.Items[i].Page == "" {
			r.Items[i].Page = l.Itm
		}
		count[r.Items[i].Page]++
	}
	r.Pages = nil
	for _, p := range l.Pages {
		if p.ID == "overview" || p.ID == "fixes" || count[p.ID] > 0 {
			r.Pages = append(r.Pages, p)
		}
	}
}
