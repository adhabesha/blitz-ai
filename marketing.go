package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------- keyword research ----------------

func suggest(ctx context.Context, q, hl, gl string) []string {
	u := "https://suggestqueries.google.com/complete/search?client=firefox&hl=" + url.QueryEscape(hl) + "&gl=" + url.QueryEscape(gl) + "&q=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	var raw []json.RawMessage
	if json.NewDecoder(resp.Body).Decode(&raw) != nil || len(raw) < 2 {
		return nil
	}
	var out []string
	json.Unmarshal(raw[1], &out)
	return out
}

type kwCluster struct {
	Name     string   `json:"name"`
	Intent   string   `json:"intent"`
	Stage    string   `json:"stage"`
	Keywords []string `json:"keywords"`
	PageIdea string   `json:"pageIdea"`
	Title    string   `json:"title"`
	Meta     string   `json:"meta"`
	Priority int      `json:"priority"`
}

func (s *server) evalKeywords(ctx context.Context, in evalIn) (*Report, error) {
	if !in.UseAI {
		return nil, userErr("Keyword research uses the free AI quota and today's AI limit is used up. Try again tomorrow.")
	}
	seed := clip(in.Query, 80)
	if len([]rune(seed)) < 3 {
		return nil, userErr("Describe the topic, product or service, for example: boutique hotel in Addis Ababa")
	}
	hl, gl := strings.ToLower(clip(in.Lang, 5)), strings.ToLower(clip(in.Country, 5))
	if hl == "" {
		hl = "en"
	}
	if gl == "" {
		gl = "us"
	}
	queries := []string{seed}
	for _, m := range []string{" near me", " price", " cost", " best", " vs", " for", " online", " reviews", " services", " in"} {
		queries = append(queries, seed+m)
	}
	for _, p := range []string{"how to ", "what is ", "why ", "best ", "cheap ", "top ", "where to "} {
		queries = append(queries, p+seed)
	}
	for ch := 'a'; ch <= 'z'; ch++ {
		queries = append(queries, seed+" "+string(ch))
	}
	seen := map[string]bool{}
	var kws []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for _, q := range queries {
		wg.Add(1)
		sem <- struct{}{}
		go func(q string) {
			defer wg.Done()
			defer func() { <-sem }()
			for _, k := range suggest(ctx, q, hl, gl) {
				k = strings.ToLower(strings.TrimSpace(k))
				mu.Lock()
				if k != "" && !seen[k] {
					seen[k] = true
					kws = append(kws, k)
				}
				mu.Unlock()
			}
		}(q)
	}
	wg.Wait()
	if len(kws) == 0 {
		return nil, userErr("Google returned no suggestions for that topic. Try a broader phrase.")
	}
	sort.Strings(kws)
	if len(kws) > 140 {
		kws = kws[:140]
	}
	prompt := fmt.Sprintf("You are a senior SEO strategist. Topic: %s. Market: country %s, language %s. Below are real Google autocomplete suggestions (no search-volume data). Group them into 8-12 keyword clusters by search intent. Return JSON only: {\"clusters\":[{\"name\":\"\",\"intent\":\"informational|commercial|transactional|navigational\",\"stage\":\"awareness|consideration|decision\",\"keywords\":[\"only keywords from the list\"],\"pageIdea\":\"one page or post to create\",\"title\":\"SEO title under 60 characters\",\"meta\":\"meta description under 155 characters\",\"priority\":3}],\"quickWins\":[\"5 specific actions\"]}. Priority is an integer 1-5 where 5 is the best opportunity. Keywords: %s", seed, gl, hl, cut(strings.Join(kws, "; "), 4200))
	txt, err := s.gemini(ctx, prompt)
	if err != nil {
		return nil, userErr("The AI is busy right now (free quota). Try again in a minute.")
	}
	var ai struct {
		Clusters  []kwCluster `json:"clusters"`
		QuickWins []string    `json:"quickWins"`
	}
	if json.Unmarshal([]byte(cleanJSON(txt)), &ai) != nil || len(ai.Clusters) == 0 {
		return nil, userErr("The AI reply could not be read. Try again.")
	}
	sort.SliceStable(ai.Clusters, func(i, j int) bool { return ai.Clusters[i].Priority > ai.Clusters[j].Priority })
	if len(ai.Clusters) > 12 {
		ai.Clusters = ai.Clusters[:12]
	}
	r := &Report{Target: seed, Meta: map[string]string{"domain": seed, "name": seed}}
	ct := Table{Title: "Keyword clusters (prioritised)", Columns: []string{"Cluster", "Intent", "Stage", "Priority", "Keywords", "Suggested page title", "Meta description"}}
	intentCount := map[string]float64{}
	intentPri := map[string][]float64{}
	var topKw []string
	var totalPri float64
	for i, cl := range ai.Clusters {
		if cl.Priority < 1 {
			cl.Priority = 1
		}
		if cl.Priority > 5 {
			cl.Priority = 5
		}
		kk := cl.Keywords
		if len(kk) > 6 {
			kk = kk[:6]
		}
		ct.Rows = append(ct.Rows, Row{Cells: []string{cl.Name, cl.Intent, cl.Stage, fmt.Sprintf("%d/5", cl.Priority), strings.Join(kk, ", "), cut(cl.Title, 70), cut(cl.Meta, 160)}})
		in2 := strings.ToLower(cl.Intent)
		if in2 == "" {
			in2 = "other"
		}
		intentCount[in2] += float64(len(cl.Keywords))
		intentPri[in2] = append(intentPri[in2], float64(cl.Priority))
		totalPri += float64(cl.Priority)
		if i < 3 {
			topKw = append(topKw, kk...)
		}
	}
	at := Table{Title: "All keyword ideas from Google suggestions", Columns: []string{"Keyword"}}
	for i, k := range kws {
		if i >= 100 {
			break
		}
		at.Rows = append(at.Rows, Row{Cells: []string{k}})
	}
	r.Tables = []Table{ct, at}
	var labels []string
	var data []float64
	for k, v := range intentCount {
		labels = append(labels, k)
		data = append(data, v)
	}
	r.Charts = []Chart{{Title: "Keywords by search intent", Type: "doughnut", Labels: labels, Datasets: []Dataset{{Name: "Keywords", Data: data}}}}
	high := 0
	for _, cl := range ai.Clusters {
		if cl.Priority >= 4 {
			high++
		}
	}
	r.Categories = map[string]int{}
	for k, v := range intentPri {
		sum := 0.0
		for _, x := range v {
			sum += x
		}
		r.Categories[k] = int(sum / float64(len(v)) * 20)
	}
	r.Score = int(math.Round(totalPri / float64(len(ai.Clusters)) * 20))
	r.KPIs = []KPI{{Label: "Keyword ideas found", Value: commas(float64(len(kws))), Note: "real Google suggestions"}, {Label: "Clusters", Value: commas(float64(len(ai.Clusters))), Note: "grouped by intent"},
		{Label: "High-priority clusters", Value: commas(float64(high)), Note: "priority 4-5", Status: ifs(high >= 3, "pass", "warn")}, {Label: "Quick wins", Value: commas(float64(len(ai.QuickWins))), Note: "actions to start now"}}
	for _, qw := range ai.QuickWins {
		if strings.TrimSpace(qw) != "" {
			r.Plan = append(r.Plan, PlanItem{Title: cut(qw, 160), Why: "Quick win from the keyword research.", Impact: 4, Effort: 2})
		}
	}
	for i, cl := range ai.Clusters {
		if i >= 4 {
			break
		}
		name := cl.PageIdea
		if name == "" {
			name = cl.Name
		}
		r.Plan = append(r.Plan, PlanItem{Title: "Create or optimise a page: " + cut(name, 90), Why: fmt.Sprintf("%s intent, priority %d/5. Suggested title: %s", cl.Intent, cl.Priority, cut(cl.Title, 70)), Impact: cl.Priority, Effort: 3, Auto: "draft:seo-content"})
	}
	if len(topKw) > 8 {
		topKw = topKw[:8]
	}
	r.Meta["queries"] = strings.Join(topKw, ", ")
	r.Notes = append(r.Notes, "Suggestions come from Google's public autocomplete, so they show what people really type. They do not include search volume. Connect Search Console for your own impression data, or add Google Ads Keyword Planner access during client setup for volumes.")
	return r, nil
}

// ---------------- competitor analysis ----------------

func scoreNum(v string) int {
	var n int
	fmt.Sscanf(v, "%d", &n)
	return n
}

func findCheck(r *Report, name string) *Check {
	for i := range r.Checks {
		if r.Checks[i].Name == name {
			return &r.Checks[i]
		}
	}
	return nil
}

type compCand struct{ Name, Domain, Why string }

// Sites that are directories, marketplaces or social networks are never direct competitors.
var notCompetitor = []string{"facebook.com", "instagram.com", "linkedin.com", "youtube.com", "tiktok.com", "twitter.com", "x.com", "pinterest.com", "wikipedia.org", "yelp.com", "tripadvisor.com", "google.com", "booking.com", "amazon.com", "alibaba.com", "olx.com", "reddit.com", "quora.com", "medium.com", "yellowpages.com", "crunchbase.com", "glassdoor.com", "indeed.com"}

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style|noscript|svg)[^>]*>.*?</(script|style|noscript|svg)>`)
	reAnyTag      = regexp.MustCompile(`(?s)<[^>]+>`)
)

func pageText(doc string, n int) string {
	t := reScriptStyle.ReplaceAllString(doc, " ")
	t = reAnyTag.ReplaceAllString(t, " ")
	return clip(strings.Join(strings.Fields(html.UnescapeString(t)), " "), n)
}

// findCompetitors reads the person's own site to learn the category and the place it serves, then asks Gemini
// (with live Google Search) for real businesses that offer the same thing in the same area.
func (s *server) findCompetitors(ctx context.Context, in evalIn) (category, location string, out []compCand, live bool, err error) {
	if env("GEMINI_API_KEY") == "" {
		return "", "", nil, false, errors.New("AI is not set up (GEMINI_API_KEY)")
	}
	target, domain, nerr := normalizeURL(in.URL)
	if nerr != nil {
		return "", "", nil, false, nerr
	}
	title, desc, text := "", "", ""
	if pg, ferr := fetch(ctx, target, 1<<20); ferr == nil {
		doc := string(pg.body)
		if m := reTitle.FindStringSubmatch(doc); m != nil {
			title = strings.TrimSpace(html.UnescapeString(m[1]))
		}
		desc = meta(doc, "description")
		text = pageText(doc, 1500)
	}
	prompt := fmt.Sprintf(`Use Google Search. A business has this website.
Domain: %s
Title: %s
Description: %s
Text from the homepage: %s
Country code given by the owner: %s
City or area given by the owner (may be empty): %s
Business type given by the owner (may be empty): %s

Step 1: work out what exactly this business sells (its category and main services) and where it operates (city or area, and country). Prefer what the owner gave, otherwise use the website.
Step 2: find 7 real DIRECT competitors: businesses with their own website that offer the same kind of products or services to customers in the same city or area (widen to the same country only if there are not enough local ones), in the same category and at a similar scale. Exclude the business itself, directories, marketplaces, social networks, news sites and Wikipedia. Only include websites you saw in the search results; never invent a domain.
Return JSON only, no other text: {"category":"short category","location":"city, country","competitors":[{"name":"business name","domain":"example.com","why":"one short line on why it competes"}]}`,
		domain, clip(title, 150), clip(desc, 300), text, clip(in.Country, 5), clip(in.Location, 80), clip(in.Topic, 120))
	cctx, cancel := context.WithTimeout(ctx, 38*time.Second)
	defer cancel()
	// Search grounding has small quotas that are counted per model, so try several models, then fall back to
	// the model's own knowledge (the competitor sites are still opened and audited, so dead or invented ones drop out).
	var models []string
	for _, m := range []string{env("GEMINI_SEARCH_MODEL"), geminiModel(), "gemini-2.5-flash", "gemini-2.5-flash-lite"} {
		dup := m == ""
		for _, x := range models {
			dup = dup || x == m
		}
		if !dup {
			models = append(models, m)
		}
	}
	var txt string
	var gerr error
	live = false
	for _, m := range models {
		if txt, gerr = s.geminiSearch(cctx, m, prompt); gerr == nil {
			live = true
			break
		}
		log.Println("competitor search with", m, "failed:", gerr)
		if cctx.Err() != nil {
			break
		}
	}
	if !live {
		cc, cc2 := context.WithTimeout(ctx, 25*time.Second)
		txt, gerr = s.gemini(cc, strings.Replace(prompt, "Use Google Search. ", "", 1))
		cc2()
		if gerr != nil {
			if errors.Is(gerr, errRate) {
				return "", "", nil, false, userErr("The AI is busy right now (daily or per-minute limit reached). Wait a minute and run it again, or type 1 to 3 competitor websites.")
			}
			return "", "", nil, false, gerr
		}
	}
	var res struct {
		Category    string `json:"category"`
		Location    string `json:"location"`
		Competitors []struct {
			Name   string `json:"name"`
			Domain string `json:"domain"`
			Why    string `json:"why"`
		} `json:"competitors"`
	}
	if json.Unmarshal([]byte(cleanJSON(txt)), &res) != nil || len(res.Competitors) == 0 {
		return "", "", nil, false, errors.New("no competitors in the answer")
	}
	seen := map[string]bool{domain: true}
	for _, c := range res.Competitors {
		_, d, derr := normalizeURL(c.Domain)
		if derr != nil || seen[d] {
			continue
		}
		skip := false
		for _, b := range notCompetitor {
			if d == b || strings.HasSuffix(d, "."+b) {
				skip = true
			}
		}
		if skip {
			continue
		}
		seen[d] = true
		out = append(out, compCand{Name: clip(c.Name, 80), Domain: d, Why: clip(c.Why, 160)})
		if len(out) == 6 {
			break
		}
	}
	if len(out) == 0 {
		return "", "", nil, false, errors.New("no usable competitor websites in the answer")
	}
	return clip(res.Category, 80), clip(res.Location, 80), out, live, nil
}

func (s *server) evalCompetitors(ctx context.Context, in evalIn) (*Report, error) {
	if strings.TrimSpace(in.URL) == "" {
		return nil, userErr("Enter your website.")
	}
	var urls []string
	urls = append(urls, in.URL)
	for _, f := range strings.FieldsFunc(in.Query, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == ';' }) {
		if strings.TrimSpace(f) != "" && len(urls) < 4 {
			urls = append(urls, f)
		}
	}
	// No competitors typed: find them by the same category, area and location.
	var auto []compCand
	var autoCat, autoLoc string
	var autoLive bool
	if len(urls) < 2 {
		var ferr error
		autoCat, autoLoc, auto, autoLive, ferr = s.findCompetitors(ctx, in)
		if ferr != nil {
			if _, ok := ferr.(userErr); ok {
				return nil, ferr
			}
			log.Println("find competitors:", ferr)
			return nil, userErr("I couldn't find competitors automatically (" + trunc(ferr.Error(), 100) + "). Type 1 to 3 competitor websites and run it again.")
		}
		for _, c := range auto {
			urls = append(urls, c.Domain)
		}
	}
	type res struct {
		rep *Report
		err error
	}
	results := make([]res, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			in2 := in
			in2.URL = u
			in2.UseAI = false
			rep, err := s.evalWebsite(ctx, in2)
			results[i] = res{rep, err}
		}(i, u)
	}
	wg.Wait()
	if results[0].err != nil {
		return nil, results[0].err
	}
	own := results[0].rep
	r := &Report{Target: own.Target + " vs competitors", Checks: own.Checks, Meta: map[string]string{}}
	for k, v := range own.Meta {
		r.Meta[k] = v
	}
	var sites []*Report
	sites = append(sites, own)
	maxSites := 4 // up to 3 competitors for typed lists, 4 for automatic ones (extra candidates are backups for unreachable sites)
	if len(auto) > 0 {
		maxSites = 5
	}
	status := map[string]string{}
	for i, x := range results[1:] {
		dom := urls[i+1]
		if len(auto) > 0 {
			dom = auto[i].Domain
		}
		switch {
		case x.err != nil:
			status[dom] = "Not compared: " + trunc(strings.TrimSuffix(x.err.Error(), "."), 90)
			r.Notes = append(r.Notes, "Could not analyse "+dom+": "+trunc(x.err.Error(), 120))
		case len(sites) >= maxSites:
			status[dom] = "Backup, not needed"
		default:
			status[dom] = "Compared"
			sites = append(sites, x.rep)
		}
	}
	if len(auto) > 0 {
		r.Meta["category"], r.Meta["location"] = autoCat, autoLoc
		ft := Table{Title: "Competitors found for you (same category and area)", Columns: []string{"Business", "Website", "Why it competes", "Status"}}
		for _, c := range auto {
			ft.Rows = append(ft.Rows, Row{Cells: []string{c.Name, c.Domain, c.Why, status[c.Domain]}})
		}
		r.Tables = append(r.Tables, ft)
		r.Notes = append(r.Notes, "Competitors were found automatically for category \""+autoCat+"\" in "+autoLoc+", "+ifs(autoLive, "using live Google Search", "from the AI's own knowledge because live search was busy, so check that they really are your competitors")+". Sites that could not be opened were replaced by the next best match. To compare against specific businesses instead, type their websites and run it again.")
	}
	if len(sites) < 2 {
		return nil, userErr("None of the competitor websites could be analysed. Check the addresses.")
	}
	cols := []string{"Check"}
	for _, st := range sites {
		cols = append(cols, st.Target)
	}
	mt := Table{Title: "Side-by-side audit", Columns: cols}
	for _, c := range own.Checks {
		row := []string{c.Name}
		for _, st := range sites {
			cc := findCheck(st, c.Name)
			if cc == nil {
				row = append(row, "-")
			} else {
				row = append(row, strings.ToUpper(cc.Status)+" "+cut(cc.Value, 22))
			}
		}
		mt.Rows = append(mt.Rows, Row{Cells: row})
	}
	pt := Table{Title: "How each site presents itself (titles and descriptions)", Columns: []string{"Site", "Title", "Meta description"}}
	scores := Chart{Title: "Overall audit score by site", Type: "bar", Datasets: []Dataset{{Name: "Score"}}}
	perf := Chart{Title: "Mobile performance by site (Lighthouse)", Type: "bar", Datasets: []Dataset{{Name: "Performance"}}}
	best, bestName := -1, ""
	for i, st := range sites {
		pt.Rows = append(pt.Rows, Row{Cells: []string{st.Target, cut(st.Meta["title"], 90), cut(st.Meta["description"], 160)}})
		scores.Labels = append(scores.Labels, st.Target)
		scores.Datasets[0].Data = append(scores.Datasets[0].Data, float64(st.Score))
		if c := findCheck(st, "Performance score (mobile)"); c != nil {
			perf.Labels = append(perf.Labels, st.Target)
			perf.Datasets[0].Data = append(perf.Datasets[0].Data, float64(scoreNum(c.Value)))
		}
		if i > 0 && st.Score > best {
			best, bestName = st.Score, st.Target
		}
	}
	r.Tables = append([]Table{mt, pt}, r.Tables...)
	r.Charts = []Chart{scores}
	if len(perf.Labels) > 1 {
		r.Charts = append(r.Charts, perf)
	}
	r.KPIs = []KPI{{Label: "Your audit score", Value: fmt.Sprintf("%d/100", own.Score), Status: rate(float64(own.Score), 75, 50)},
		{Label: "Best competitor", Value: fmt.Sprintf("%d/100", best), Note: bestName},
		{Label: "Gap to the best", Value: fmt.Sprintf("%+d", own.Score-best), Note: "positive means you lead", Status: ifs(own.Score >= best, "pass", "fail")}}
	adv := 0
	n := 0
	for _, c := range own.Checks {
		if c.Status == "pass" || c.Impact < 2 {
			if c.Status == "pass" {
				allFail := true
				for _, st := range sites[1:] {
					if cc := findCheck(st, c.Name); cc != nil && cc.Status == "pass" {
						allFail = false
					}
				}
				if allFail && len(sites) > 1 {
					adv++
				}
			}
			continue
		}
		var passing []string
		for _, st := range sites[1:] {
			if cc := findCheck(st, c.Name); cc != nil && cc.Status == "pass" {
				passing = append(passing, st.Target)
			}
		}
		if len(passing) > 0 && n < 8 {
			n++
			r.Plan = append(r.Plan, PlanItem{Title: "Match competitors on: " + c.Name, Why: "Passing on " + strings.Join(passing, ", ") + ". Yours: " + c.Value + ". " + c.Fix, Impact: c.Impact, Effort: 2})
		}
	}
	r.KPIs = append(r.KPIs, KPI{Label: "Your advantages", Value: fmt.Sprintf("%d", adv), Note: "checks you pass and every competitor fails", Status: ifs(adv > 0, "pass", "")})
	r.Categories = own.Categories
	r.Score = own.Score
	if len(r.Plan) == 0 {
		r.Plan = append(r.Plan, PlanItem{Title: "Keep your lead: re-run this comparison monthly", Why: "No gap found where competitors beat you.", Impact: 2, Effort: 1})
	}
	r.Notes = append(r.Notes, "Compared with the same audit on every site: on-page SEO, speed, structured data, crawl access and AI readiness. Backlink and traffic estimates need paid data sources and are added per client during setup.")
	return r, nil
}

// ---------------- ads planner ----------------

type adsAI struct {
	Google struct {
		Campaign string `json:"campaign"`
		AdGroups []struct {
			Name     string `json:"name"`
			Keywords []struct {
				Text  string `json:"text"`
				Match string `json:"match"`
			} `json:"keywords"`
			Headlines    []string `json:"headlines"`
			Descriptions []string `json:"descriptions"`
		} `json:"adGroups"`
		Negatives []string `json:"negatives"`
		Sitelinks []struct {
			Text  string `json:"text"`
			Desc1 string `json:"desc1"`
			Desc2 string `json:"desc2"`
		} `json:"sitelinks"`
		Callouts []string `json:"callouts"`
	} `json:"google"`
	Meta struct {
		Audiences []string `json:"audiences"`
		Ads       []struct {
			Primary     string `json:"primary"`
			Headline    string `json:"headline"`
			Description string `json:"description"`
			CTA         string `json:"cta"`
		} `json:"ads"`
	} `json:"meta"`
	Budget struct {
		GooglePct float64 `json:"googlePct"`
		MetaPct   float64 `json:"metaPct"`
		Why       string  `json:"why"`
	} `json:"budget"`
	Targets struct {
		CTR  string `json:"ctr"`
		CVR  string `json:"cvr"`
		Note string `json:"note"`
	} `json:"targets"`
}

func fmtKw(text, match string) string {
	switch strings.ToLower(match) {
	case "exact":
		return "[" + text + "]"
	case "phrase":
		return "\"" + text + "\""
	}
	return text
}

func (s *server) evalAds(ctx context.Context, in evalIn) (*Report, error) {
	if !in.UseAI {
		return nil, userErr("The ads planner uses the free AI quota and today's AI limit is used up. Try again tomorrow.")
	}
	offer := clip(in.Query, 200)
	if len([]rune(offer)) < 5 {
		return nil, userErr("Describe the product or service you want to advertise.")
	}
	goal := strings.ToLower(clip(in.Goal, 20))
	if goal == "" {
		goal = "leads"
	}
	site := ""
	if in.URL != "" {
		if _, dom, err := normalizeURL(in.URL); err == nil {
			site = dom
		}
	}
	prompt := fmt.Sprintf("You are a senior paid-media strategist. Plan a Google Search campaign and Meta (Facebook and Instagram) ads for: %s. Goal: %s. Website: %s. Country: %s, language: %s. Monthly budget: %.0f USD. Return JSON only: {\"google\":{\"campaign\":\"name\",\"adGroups\":[{\"name\":\"\",\"keywords\":[{\"text\":\"\",\"match\":\"exact|phrase|broad\"}],\"headlines\":[15 headlines, each at most 30 characters],\"descriptions\":[4 descriptions, each at most 90 characters]}],\"negatives\":[10 negative keywords],\"sitelinks\":[{\"text\":\"max 25 chars\",\"desc1\":\"max 35\",\"desc2\":\"max 35\"}],\"callouts\":[6 callouts, max 25 chars]},\"meta\":{\"audiences\":[4 audience ideas],\"ads\":[{\"primary\":\"primary text, under 125 characters\",\"headline\":\"max 40 characters\",\"description\":\"max 30 characters\",\"cta\":\"LEARN_MORE|SIGN_UP|SHOP_NOW|CONTACT_US|BOOK_NOW\"}]},\"budget\":{\"googlePct\":60,\"metaPct\":40,\"why\":\"one sentence\"},\"targets\":{\"ctr\":\"expected range\",\"cvr\":\"expected range\",\"note\":\"one sentence\"}}. Use 2-3 ad groups and 3-4 Meta ads. Never invent prices, awards or statistics.", offer, goal, ifs(site == "", "not provided", site), clip(in.Country, 5), clip(in.Lang, 5), in.Budget)
	txt, err := s.gemini(ctx, prompt)
	if err != nil {
		return nil, userErr("The AI is busy right now (free quota). Try again in a minute.")
	}
	var a adsAI
	if json.Unmarshal([]byte(cleanJSON(txt)), &a) != nil || len(a.Google.AdGroups) == 0 {
		return nil, userErr("The AI reply could not be read. Try again.")
	}
	r := &Report{Target: cut(offer, 50), Meta: map[string]string{"name": offer, "domain": site}}
	hl := Table{Title: "Google Search ads: headlines (max 30 characters)", Columns: []string{"Ad group", "Headline", "Characters"}}
	ds := Table{Title: "Google Search ads: descriptions (max 90 characters)", Columns: []string{"Ad group", "Description", "Characters"}}
	kw := Table{Title: "Google keywords by ad group (Ads Editor format)", Columns: []string{"Ad group", "Keyword", "Match type"}}
	nHead, nDesc, nKw := 0, 0, 0
	for _, g := range a.Google.AdGroups {
		for _, h := range g.Headlines {
			h = cut(strings.TrimSpace(h), 30)
			if h != "" {
				hl.Rows = append(hl.Rows, Row{Cells: []string{g.Name, h, fmt.Sprint(len([]rune(h)))}})
				nHead++
			}
		}
		for _, d := range g.Descriptions {
			d = cut(strings.TrimSpace(d), 90)
			if d != "" {
				ds.Rows = append(ds.Rows, Row{Cells: []string{g.Name, d, fmt.Sprint(len([]rune(d)))}})
				nDesc++
			}
		}
		for _, k := range g.Keywords {
			if strings.TrimSpace(k.Text) != "" {
				kw.Rows = append(kw.Rows, Row{Cells: []string{g.Name, fmtKw(strings.TrimSpace(k.Text), k.Match), strings.ToLower(k.Match)}})
				nKw++
			}
		}
	}
	neg := Table{Title: "Google negative keywords", Columns: []string{"Negative keyword"}}
	for _, n := range a.Google.Negatives {
		if strings.TrimSpace(n) != "" {
			neg.Rows = append(neg.Rows, Row{Cells: []string{strings.TrimSpace(n)}})
		}
	}
	sl := Table{Title: "Google sitelinks and callouts (extensions)", Columns: []string{"Type", "Text", "Line 1", "Line 2"}}
	for _, x := range a.Google.Sitelinks {
		sl.Rows = append(sl.Rows, Row{Cells: []string{"Sitelink", cut(x.Text, 25), cut(x.Desc1, 35), cut(x.Desc2, 35)}})
	}
	for _, x := range a.Google.Callouts {
		sl.Rows = append(sl.Rows, Row{Cells: []string{"Callout", cut(x, 25), "", ""}})
	}
	mt := Table{Title: "Meta (Facebook and Instagram) ad variations", Columns: []string{"Primary text", "Headline (max 40)", "Description (max 30)", "Call to action"}}
	for _, m := range a.Meta.Ads {
		mt.Rows = append(mt.Rows, Row{Cells: []string{cut(m.Primary, 125), cut(m.Headline, 40), cut(m.Description, 30), strings.ToUpper(m.CTA)}})
	}
	at := Table{Title: "Meta audience ideas", Columns: []string{"Audience"}}
	for _, x := range a.Meta.Audiences {
		at.Rows = append(at.Rows, Row{Cells: []string{x}})
	}
	r.Tables = []Table{hl, ds, kw, neg, sl, mt, at}
	gp, mp := a.Budget.GooglePct, a.Budget.MetaPct
	if gp+mp <= 0 {
		gp, mp = 60, 40
	}
	tot := gp + mp
	gp, mp = gp/tot*100, mp/tot*100
	r.Charts = []Chart{{Title: "Suggested budget split (%)", Type: "doughnut", Labels: []string{"Google Search", "Meta"}, Datasets: []Dataset{{Name: "Budget", Data: []float64{math.Round(gp), math.Round(mp)}}}}}
	r.KPIs = []KPI{{Label: "Headlines", Value: fmt.Sprint(nHead), Note: "30 characters max each"}, {Label: "Keywords", Value: fmt.Sprint(nKw), Note: fmt.Sprintf("%d ad groups", len(a.Google.AdGroups))},
		{Label: "Meta ad variations", Value: fmt.Sprint(len(a.Meta.Ads)), Note: "to A/B test"}}
	if in.Budget > 0 {
		r.KPIs = append(r.KPIs, KPI{Label: "Google budget", Value: "$" + commas(in.Budget*gp/100), Note: fmt.Sprintf("%.0f%% of $%s", gp, commas(in.Budget))}, KPI{Label: "Meta budget", Value: "$" + commas(in.Budget*mp/100), Note: fmt.Sprintf("%.0f%% of $%s", mp, commas(in.Budget))})
	}
	if a.Targets.CTR != "" || a.Targets.CVR != "" {
		r.KPIs = append(r.KPIs, KPI{Label: "Target CTR / conversion", Value: cut(a.Targets.CTR, 14) + " / " + cut(a.Targets.CVR, 14), Note: cut(a.Targets.Note, 80)})
	}
	const AC = "Ad completeness"
	add := func(c Check) { r.Checks = append(r.Checks, c) }
	add(chk(AC, "Headlines (15 recommended)", rate(float64(nHead), 12, 6), fmt.Sprint(nHead), "Responsive search ads need up to 15 headlines; ask Blitz AI to regenerate for more.", 4))
	add(chk(AC, "Descriptions (4 recommended)", rate(float64(nDesc), 4, 2), fmt.Sprint(nDesc), "Provide 4 descriptions per ad group.", 3))
	add(chk(AC, "Keywords (10+ across 2+ ad groups)", rate(float64(nKw), 10, 5), fmt.Sprint(nKw), "Add more specific keywords per theme.", 4))
	add(chk(AC, "Negative keywords (5+)", rate(float64(len(neg.Rows)), 5, 2), fmt.Sprint(len(neg.Rows)), "Negatives stop wasted spend on irrelevant searches.", 3))
	add(chk(AC, "Meta ad variations (3+)", rate(float64(len(a.Meta.Ads)), 3, 2), fmt.Sprint(len(a.Meta.Ads)), "Test at least three creatives and two audiences.", 3))
	r.finish()
	goalNote := "Before spending: make sure your landing page and GA4 conversion (key event) tracking are working."
	r.Plan = []PlanItem{
		{Title: "Set up conversion tracking before launch", Why: goalNote, Impact: 5, Effort: 2},
		{Title: "Create the Google campaign '" + cut(a.Google.Campaign, 40) + "' with the ad groups, keywords and negatives", Why: "Copy each table into Google Ads Editor or the Ads interface.", Impact: 5, Effort: 3},
		{Title: "Add the extensions (sitelinks and callouts) and point every ad to a relevant landing page", Why: "Extensions raise click-through rate and use more of the page.", Impact: 3, Effort: 2},
		{Title: "Launch the Meta ads with 2 audiences x 3 creatives", Why: "Let each variation spend equally for 7 days before judging.", Impact: 4, Effort: 3},
		{Title: "After 14 days: pause the weakest 30% of headlines and ads, then shift budget to winners", Why: "Use CTR and cost per conversion from the campaign reports.", Impact: 4, Effort: 2},
	}
	r.Notes = append(r.Notes, "This is campaign planning and ad copy. Live campaign performance import uses the Google Ads API (developer token) and the Meta Marketing API (ads_read); those connections are set up per client during onboarding. Review every ad against each platform's policies before publishing.")
	return r, nil
}
