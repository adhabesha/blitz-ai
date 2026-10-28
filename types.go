package main

import (
	"strings"
	"time"
)

type userErr string

func (e userErr) Error() string { return string(e) }

type Check struct {
	Name     string `json:"name" firestore:"name"`
	Category string `json:"category" firestore:"category"`
	Status   string `json:"status" firestore:"status"` // pass | warn | fail
	Value    string `json:"value" firestore:"value"`
	Fix      string `json:"fix" firestore:"fix"`
	Impact   int    `json:"impact" firestore:"impact"` // 1-5
}

type KPI struct {
	Label  string `json:"label" firestore:"label"`
	Value  string `json:"value" firestore:"value"`
	Note   string `json:"note" firestore:"note"`
	Status string `json:"status" firestore:"status"`
}

type Point struct {
	Label string  `json:"label" firestore:"label"`
	Value float64 `json:"value" firestore:"value"`
}

type Dataset struct {
	Name string    `json:"name" firestore:"name"`
	Data []float64 `json:"data" firestore:"data"`
}

type Chart struct {
	Title    string    `json:"title" firestore:"title"`
	Type     string    `json:"type" firestore:"type"`
	Labels   []string  `json:"labels" firestore:"labels"`
	Datasets []Dataset `json:"datasets" firestore:"datasets"`
}

type AIInsights struct {
	Summary      string   `json:"summary" firestore:"summary"`
	Strengths    []string `json:"strengths" firestore:"strengths"`
	Improvements []string `json:"improvements" firestore:"improvements"`
	Plan         []string `json:"plan" firestore:"plan"`
	Ideas        []string `json:"ideas" firestore:"ideas"`
}

type Report struct {
	ID         string         `json:"id" firestore:"-"`
	Kind       string         `json:"kind" firestore:"kind"`
	Target     string         `json:"target" firestore:"target"`
	Score      int            `json:"score" firestore:"score"`
	Categories map[string]int `json:"categories" firestore:"categories"`
	KPIs       []KPI          `json:"kpis" firestore:"kpis"`
	Checks     []Check        `json:"checks" firestore:"checks"`
	Charts     []Chart        `json:"charts" firestore:"charts"`
	AI         *AIInsights    `json:"ai,omitempty" firestore:"ai,omitempty"`
	Notes      []string       `json:"notes" firestore:"notes"`
	AIUsed     bool           `json:"aiUsed" firestore:"aiUsed"`
	CreatedAt  time.Time      `json:"createdAt" firestore:"createdAt"`
}

type evalIn struct {
	Kind  string `json:"kind"`
	UseAI bool   `json:"useAI"`
	// website
	URL string `json:"url"`
	// social
	Platform      string  `json:"platform"`
	Handle        string  `json:"handle"`
	Followers     float64 `json:"followers"`
	PrevFollowers float64 `json:"prevFollowers"`
	Following     float64 `json:"following"`
	Posts30       float64 `json:"posts30"`
	Likes         float64 `json:"likes"`
	Comments      float64 `json:"comments"`
	Shares        float64 `json:"shares"`
	Views         float64 `json:"views"`
	Bio           string  `json:"bio"`
	Captions      string  `json:"captions"`
	Goal          string  `json:"goal"`
	// app
	Store     string  `json:"store"`
	Query     string  `json:"query"`
	Title     string  `json:"title"`
	Rating    float64 `json:"rating"`
	Ratings   float64 `json:"ratings"`
	DaysSince float64 `json:"daysSince"`
	DescLen   float64 `json:"descLen"`
	Shots     float64 `json:"shots"`
}

func chk(cat, name, status, value, fix string, impact int) Check {
	return Check{Name: name, Category: cat, Status: status, Value: value, Fix: fix, Impact: impact}
}

func ifs(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

// higher is better
func rate(v, good, ok float64) string {
	if v >= good {
		return "pass"
	}
	if v >= ok {
		return "warn"
	}
	return "fail"
}

// lower is better
func low(v, good, ok float64) string {
	if v <= good {
		return "pass"
	}
	if v <= ok {
		return "warn"
	}
	return "fail"
}

func clip(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r == '"' || r == '\n' || r == '\r' || r == '\\' || r == '\t' {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n])
	}
	return s
}

func kpiFrom(c Check, label, note string) KPI {
	return KPI{Label: label, Value: c.Value, Note: note, Status: c.Status}
}

// finish computes weighted category and overall scores from the checks.
func (r *Report) finish() {
	w := map[string]float64{}
	t := map[string]float64{}
	var tw, ts float64
	for _, c := range r.Checks {
		v := 0.0
		switch c.Status {
		case "pass":
			v = 1
		case "warn":
			v = 0.5
		}
		wt := float64(c.Impact)
		if wt < 1 {
			wt = 1
		}
		w[c.Category] += wt
		t[c.Category] += wt * v
		tw += wt
		ts += wt * v
	}
	r.Categories = map[string]int{}
	for k := range w {
		r.Categories[k] = int(t[k]/w[k]*100 + 0.5)
	}
	if tw > 0 {
		r.Score = int(ts/tw*100 + 0.5)
	}
}
