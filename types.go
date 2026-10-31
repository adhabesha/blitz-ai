package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
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
	Page     string `json:"page" firestore:"page"`
}

type KPI struct {
	Label  string `json:"label" firestore:"label"`
	Value  string `json:"value" firestore:"value"`
	Note   string `json:"note" firestore:"note"`
	Status string `json:"status" firestore:"status"`
	Page   string    `json:"page" firestore:"page"`
	Spark  []float64 `json:"spark" firestore:"spark"`
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
	Tips     []string  `json:"tips" firestore:"tips"`
	Page     string    `json:"page" firestore:"page"`
}

type Row struct {
	Cells []string `json:"cells" firestore:"cells"`
}

type Table struct {
	Title   string   `json:"title" firestore:"title"`
	Columns []string `json:"columns" firestore:"columns"`
	Rows    []Row    `json:"rows" firestore:"rows"`
	Page    string   `json:"page" firestore:"page"`
}

type PlanItem struct {
	Title  string `json:"title" firestore:"title"`
	Why    string `json:"why" firestore:"why"`
	Impact int    `json:"impact" firestore:"impact"`
	Effort int    `json:"effort" firestore:"effort"`
	Auto   string `json:"auto" firestore:"auto"` // e.g. optimize-video:<id>
}

type PageDef struct {
	ID    string `json:"id" firestore:"id"`
	Title string `json:"title" firestore:"title"`
}

type Item struct {
	ID        string             `json:"id" firestore:"id"`
	Title     string             `json:"title" firestore:"title"`
	Thumb     string             `json:"thumb" firestore:"thumb"`
	URL       string             `json:"url" firestore:"url"`
	Published string             `json:"published" firestore:"published"`
	Kind      string             `json:"kind" firestore:"kind"`
	Page      string             `json:"page" firestore:"page"`
	Metrics   map[string]float64 `json:"metrics" firestore:"metrics"`
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
	Tables     []Table        `json:"tables" firestore:"tables"`
	Plan       []PlanItem     `json:"plan" firestore:"plan"`
	Pages      []PageDef      `json:"pages" firestore:"pages"`
	Items      []Item         `json:"items" firestore:"items"`
	AI         *AIInsights    `json:"ai,omitempty" firestore:"ai,omitempty"`
	Notes      []string       `json:"notes" firestore:"notes"`
	Meta       map[string]string `json:"meta" firestore:"meta"`
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
	// google (connected) and youtube
	Site     string `json:"site"`
	Property string `json:"property"`
	Channel  bool   `json:"channel"`
	ExactSubs float64 `json:"exactSubs"`
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

func commas(v float64) string {
	n := int64(math.Round(v))
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func human(v float64) string {
	if math.Abs(v) >= 1e6 {
		return fmt.Sprintf("%.2fM", v/1e6)
	}
	return commas(v)
}

func fmtHMS(sec float64) string {
	s := int64(math.Round(sec))
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, (s%3600)/60, s%60)
}

func atof(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func delta(cur, prev float64) float64 {
	if prev == 0 {
		return 0
	}
	return (cur - prev) / prev * 100
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func trunc(s string, n int) string {
	rs := []rune(s)
	if len(rs) > n {
		return string(rs[:n-1]) + "\u2026"
	}
	return s
}

// forecast fits a straight line to y and projects it forward (standard trend projection).
func forecast(y []float64, days int) (fitted []float64, total, r2 float64) {
	n := float64(len(y))
	if n < 7 {
		return nil, 0, 0
	}
	var sx, sy, sxx, sxy float64
	for i, v := range y {
		x := float64(i)
		sx += x
		sy += v
		sxx += x * x
		sxy += x * v
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return nil, 0, 0
	}
	b := (n*sxy - sx*sy) / den
	a := (sy - b*sx) / n
	mean := sy / n
	var ssTot, ssRes float64
	for i, v := range y {
		f := a + b*float64(i)
		ssRes += (v - f) * (v - f)
		ssTot += (v - mean) * (v - mean)
	}
	if ssTot > 0 {
		r2 = 1 - ssRes/ssTot
	}
	for i := 0; i < len(y)+days; i++ {
		f := math.Max(0, a+b*float64(i))
		fitted = append(fitted, math.Round(f*10)/10)
		if i >= len(y) {
			total += f
		}
	}
	return
}

// lineForecast builds an "actual + trend/forecast" chart. dates are YYYY-MM-DD.
func lineForecast(title string, dates []string, y []float64, days int) (Chart, float64, float64, bool) {
	fit, total, r2 := forecast(y, days)
	if fit == nil || len(dates) != len(y) {
		return Chart{}, 0, 0, false
	}
	ch := Chart{Title: title, Type: "line"}
	for _, d := range dates {
		if len(d) >= 10 {
			ch.Labels = append(ch.Labels, d[5:10])
		} else {
			ch.Labels = append(ch.Labels, d)
		}
	}
	if last, err := time.Parse("2006-01-02", dates[len(dates)-1]); err == nil {
		for k := 1; k <= days; k++ {
			ch.Labels = append(ch.Labels, last.AddDate(0, 0, k).Format("01-02"))
		}
	} else {
		for k := 1; k <= days; k++ {
			ch.Labels = append(ch.Labels, fmt.Sprintf("+%dd", k))
		}
	}
	ch.Datasets = []Dataset{{Name: "Actual", Data: y}, {Name: "Trend + " + strconv.Itoa(days) + "-day forecast", Data: fit}}
	return ch, total, r2, true
}

func confidence(r2 float64) string {
	switch {
	case r2 >= 0.6:
		return "high"
	case r2 >= 0.25:
		return "medium"
	}
	return "low"
}
