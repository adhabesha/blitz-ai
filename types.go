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
	// Graded checks carry a continuous 0..1 score instead of the 3-step pass/warn/fail value.
	Pts    float64 `json:"pts,omitempty" firestore:"pts,omitempty"`
	Graded bool    `json:"graded,omitempty" firestore:"graded,omitempty"`
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
	Links    []string  `json:"links,omitempty" firestore:"links,omitempty"` // per point: opens the video or post (same order as Labels)
	Open     string    `json:"open,omitempty" firestore:"open,omitempty"`   // where Links lead, e.g. "YouTube Studio"
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
	Text      string             `json:"text,omitempty" firestore:"text,omitempty"` // full caption of a post (Title is shortened)
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
	// Confidence (0-100) says how much data the score rests on; low values pull the score toward the middle.
	Confidence int `json:"confidence,omitempty" firestore:"confidence,omitempty"`
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
	// per-post samples from a connected account (engagement = likes+comments+shares; views only for posts that report them)
	PostEng   []float64 `json:"-"`
	PostViews []float64 `json:"-"`
	// google (connected) and youtube
	Site     string `json:"site"`
	Property string `json:"property"`
	Channel  bool   `json:"channel"`
	ExactSubs float64 `json:"exactSubs"`
	Budget    float64 `json:"budget"`
	Country   string  `json:"country"`
	Topic     string  `json:"topic"`    // optional business type / category hint (competitor finder)
	Location  string  `json:"location"` // optional city or area hint (competitor finder)
	Lang      string  `json:"lang"`
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

// grade turns a measurement into a status plus a smooth 0..1 score:
// at "good" or above = 1, at "ok" = 0.5, at 0 = 0, linear in between.
func grade(v, good, ok float64) (string, float64) {
	if math.IsNaN(v) || v < 0 {
		v = 0
	}
	switch {
	case v >= good:
		return "pass", 1
	case v >= ok:
		if good <= ok {
			return "warn", 0.5
		}
		return "warn", 0.5 + 0.5*(v-ok)/(good-ok)
	default:
		if ok <= 0 {
			return "fail", 0
		}
		return "fail", 0.5 * v / ok
	}
}

// chkG is chk with smooth scoring.
func chkG(cat, name string, v, good, ok float64, value, fix string, impact int) Check {
	st, p := grade(v, good, ok)
	c := chk(cat, name, st, value, fix, impact)
	c.Pts, c.Graded = p, true
	return c
}

// ratioText shows ratios above 100% as a multiple so 1419% never appears as a "rate".
func ratioText(pct float64, of string) string {
	if pct >= 100 {
		return fmt.Sprintf("%.1f× your %s", pct/100, of)
	}
	return fmt.Sprintf("%.0f%% of your %s", pct, of)
}

func clampF(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// confLabel names a confidence value.
func confLabel(c float64) string {
	switch {
	case c >= 0.75:
		return "High"
	case c >= 0.45:
		return "Medium"
	}
	return "Low"
}

// finishWith scores the report and then pulls every score toward the neutral middle (50)
// in proportion to how little data there is. conf is 0..1.
func (r *Report) finishWith(conf float64) {
	r.finish()
	conf = clampF(conf, 0, 1)
	r.Confidence = int(conf*100 + 0.5)
	if conf >= 0.999 {
		return
	}
	shrink := func(v int) int { return int(conf*float64(v) + (1-conf)*50 + 0.5) }
	r.Score = shrink(r.Score)
	for k, v := range r.Categories {
		r.Categories[k] = shrink(v)
	}
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
	v := c.Value
	if i := strings.Index(v, " ("); i > 0 { // the detail in brackets belongs in the note, not the big number
		v = v[:i]
	}
	return KPI{Label: label, Value: v, Note: note, Status: c.Status}
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
		if c.Graded {
			v = c.Pts
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
