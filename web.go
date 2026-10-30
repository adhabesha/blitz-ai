package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// safeClient refuses to connect to private/loopback addresses (SSRF protection).
var safeClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil || len(ips) == 0 {
				return nil, errors.New("cannot resolve host")
			}
			for _, ip := range ips {
				if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
					return nil, errors.New("blocked address")
				}
			}
			d := net.Dialer{Timeout: 8 * time.Second}
			return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	},
}

type fetched struct {
	status int
	body   []byte
	final  string
	dur    time.Duration
}

func fetch(ctx context.Context, u string, max int64) (*fetched, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "BlitzAI-Audit/1.0")
	start := time.Now()
	resp, err := safeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, max))
	return &fetched{status: resp.StatusCode, body: b, final: resp.Request.URL.String(), dur: time.Since(start)}, nil
}

func normalizeURL(in string) (string, string, error) {
	in = strings.TrimSpace(in)
	if in == "" || len(in) > 200 {
		return "", "", errors.New("Enter a website address")
	}
	if !strings.HasPrefix(in, "http://") && !strings.HasPrefix(in, "https://") {
		in = "https://" + in
	}
	u, err := url.Parse(in)
	if err != nil || u.Host == "" || !strings.Contains(u.Host, ".") {
		return "", "", errors.New("That doesn't look like a valid website address")
	}
	return u.String(), strings.TrimPrefix(strings.ToLower(u.Hostname()), "www."), nil
}

var (
	reTitle   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reH1      = regexp.MustCompile(`(?i)<h1[\s>]`)
	reImg     = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	reMeta    = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	reLink    = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	reLd      = regexp.MustCompile(`(?i)<script[^>]+application/ld\+json`)
	reLang    = regexp.MustCompile(`(?i)<html[^>]*\blang\s*=`)
	attrCache = map[string]*regexp.Regexp{}
	attrMu    sync.Mutex
)

func attr(tag, name string) string {
	attrMu.Lock()
	re, ok := attrCache[name]
	if !ok {
		re = regexp.MustCompile(`(?is)[\s"']` + name + `\s*=\s*("([^"]*)"|'([^']*)')`)
		attrCache[name] = re
	}
	attrMu.Unlock()
	m := re.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	if m[2] != "" {
		return m[2]
	}
	return m[3]
}

func meta(doc, key string) string {
	for _, t := range reMeta.FindAllString(doc, -1) {
		k := strings.ToLower(attr(t, "name"))
		if k == "" {
			k = strings.ToLower(attr(t, "property"))
		}
		if k == key {
			return strings.TrimSpace(html.UnescapeString(attr(t, "content")))
		}
	}
	return ""
}

func canonical(doc string) string {
	for _, t := range reLink.FindAllString(doc, -1) {
		if strings.Contains(strings.ToLower(attr(t, "rel")), "canonical") {
			return attr(t, "href")
		}
	}
	return ""
}

func okText(f *fetched) bool {
	if f == nil || f.status != 200 {
		return false
	}
	b := strings.TrimSpace(string(f.body))
	return b != "" && !strings.HasPrefix(b, "<")
}

func blockedBots(robots string) []string {
	out := map[string]bool{}
	var group []string
	lastAgent := false
	for _, line := range strings.Split(robots, "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		low := strings.ToLower(line)
		switch {
		case strings.HasPrefix(low, "user-agent:"):
			if !lastAgent {
				group = nil
			}
			group = append(group, strings.TrimSpace(low[len("user-agent:"):]))
			lastAgent = true
		case strings.HasPrefix(low, "disallow:"):
			lastAgent = false
			if strings.TrimSpace(low[len("disallow:"):]) == "/" {
				for _, g := range group {
					out[g] = true
				}
			}
		default:
			if line != "" {
				lastAgent = false
			}
		}
	}
	var res []string
	for _, b := range []string{"gptbot", "claudebot", "perplexitybot", "google-extended", "ccbot"} {
		if out[b] || out["*"] {
			res = append(res, b)
		}
	}
	return res
}

type psiResult struct {
	OK                      bool
	Perf, SEO, A11y, BP     int
	LCP, CLS, TBT, FCP      float64
}

func (s *server) psi(ctx context.Context, target string) psiResult {
	q := url.Values{}
	q.Set("url", target)
	q.Set("strategy", "mobile")
	for _, c := range []string{"performance", "seo", "accessibility", "best-practices"} {
		q.Add("category", c)
	}
	if k := os.Getenv("PSI_API_KEY"); k != "" {
		q.Set("key", k)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/pagespeedonline/v5/runPagespeed?"+q.Encode(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return psiResult{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return psiResult{}
	}
	var d struct {
		LighthouseResult struct {
			Categories map[string]struct {
				Score float64 `json:"score"`
			} `json:"categories"`
			Audits map[string]struct {
				NumericValue float64 `json:"numericValue"`
			} `json:"audits"`
		} `json:"lighthouseResult"`
	}
	if json.NewDecoder(resp.Body).Decode(&d) != nil || len(d.LighthouseResult.Categories) == 0 {
		return psiResult{}
	}
	c, a := d.LighthouseResult.Categories, d.LighthouseResult.Audits
	pct := func(k string) int { return int(c[k].Score*100 + 0.5) }
	return psiResult{
		OK: true, Perf: pct("performance"), SEO: pct("seo"), A11y: pct("accessibility"), BP: pct("best-practices"),
		LCP: a["largest-contentful-paint"].NumericValue, CLS: a["cumulative-layout-shift"].NumericValue,
		TBT: a["total-blocking-time"].NumericValue, FCP: a["first-contentful-paint"].NumericValue,
	}
}

func (s *server) evalWebsite(ctx context.Context, in evalIn) (*Report, error) {
	target, domain, err := normalizeURL(in.URL)
	if err != nil {
		return nil, userErr(err.Error())
	}
	u, _ := url.Parse(target)
	origin := u.Scheme + "://" + u.Host
	page, err := fetch(ctx, target, 2<<20)
	if err != nil {
		return nil, userErr("Could not reach that website. Check the address and try again.")
	}
	doc := string(page.body)
	title := ""
	if m := reTitle.FindStringSubmatch(doc); m != nil {
		title = strings.TrimSpace(html.UnescapeString(m[1]))
	}
	desc := meta(doc, "description")

	var wg sync.WaitGroup
	var robots, sitemap, llms *fetched
	var ps psiResult
	var vis *visResult
	var visErr error
	wg.Add(4)
	go func() { defer wg.Done(); robots, _ = fetch(ctx, origin+"/robots.txt", 200<<10) }()
	go func() { defer wg.Done(); sitemap, _ = fetch(ctx, origin+"/sitemap.xml", 64<<10) }()
	go func() { defer wg.Done(); llms, _ = fetch(ctx, origin+"/llms.txt", 32<<10) }()
	go func() {
		defer wg.Done()
		c, cancel := context.WithTimeout(ctx, 50*time.Second)
		defer cancel()
		ps = s.psi(c, target)
	}()
	if in.UseAI {
		wg.Add(1)
		go func() { defer wg.Done(); vis, visErr = s.aiVisibility(ctx, domain, title, desc) }()
	}
	wg.Wait()

	r := &Report{Target: domain, Meta: map[string]string{"title": clip(title, 200), "description": clip(desc, 400), "url": target, "domain": domain}}
	add := func(c Check) { r.Checks = append(r.Checks, c) }
	const T, S, C, P, A = "Technical", "On-page SEO", "Crawl & AI readiness", "Performance", "Accessibility"

	// Technical
	https := strings.HasPrefix(page.final, "https://")
	add(chk(T, "HTTPS", ifs(https, "pass", "fail"), ifs(https, "Secure connection", "Not served over HTTPS"), "Enable a free TLS certificate (Cloudflare or Let's Encrypt) and redirect all HTTP traffic to HTTPS.", 5))
	add(chk(T, "Page loads (HTTP 200)", ifs(page.status == 200, "pass", "fail"), fmt.Sprintf("Status %d", page.status), "Make sure the main URL returns 200 without errors.", 5))
	ms := float64(page.dur.Milliseconds())
	add(chk(T, "Server response time", low(ms, 800, 2000), fmt.Sprintf("%.0f ms", ms), "Use caching or a CDN (Cloudflare free) and reduce server work for the first request.", 4))
	kb := float64(len(page.body)) / 1024
	add(chk(T, "HTML size", low(kb, 300, 1000), fmt.Sprintf("%.0f KB", kb), "Trim inline scripts/styles and unused markup; load heavy parts lazily.", 2))

	// On-page SEO
	tl := utf8.RuneCountInString(title)
	add(chk(S, "Title tag", ifs(tl == 0, "fail", ifs(tl >= 30 && tl <= 60, "pass", "warn")), fmt.Sprintf("%d characters", tl), "Write a unique title of 30-60 characters with your main keyword near the start.", 5))
	dl := utf8.RuneCountInString(desc)
	add(chk(S, "Meta description", ifs(dl == 0, "fail", ifs(dl >= 70 && dl <= 160, "pass", "warn")), fmt.Sprintf("%d characters", dl), "Add a compelling 70-160 character description that answers what the page offers and why click.", 4))
	h1 := len(reH1.FindAllString(doc, -1))
	add(chk(S, "H1 heading", ifs(h1 == 1, "pass", ifs(h1 == 0, "fail", "warn")), fmt.Sprintf("%d found", h1), "Use exactly one H1 that states the page's main topic.", 4))
	can := canonical(doc)
	add(chk(S, "Canonical tag", ifs(can != "", "pass", "warn"), ifs(can != "", "Present", "Missing"), "Add <link rel=\"canonical\"> to avoid duplicate-content confusion.", 2))
	add(chk(S, "Mobile viewport", ifs(meta(doc, "viewport") != "", "pass", "fail"), ifs(meta(doc, "viewport") != "", "Present", "Missing"), "Add <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">.", 4))
	add(chk(S, "Language attribute", ifs(reLang.MatchString(doc), "pass", "warn"), ifs(reLang.MatchString(doc), "Present", "Missing"), "Set <html lang=\"en\"> (or your language).", 1))
	ogOK := meta(doc, "og:title") != "" && meta(doc, "og:image") != ""
	ogAny := meta(doc, "og:title") != "" || meta(doc, "og:image") != ""
	add(chk(S, "Open Graph (social previews)", ifs(ogOK, "pass", ifs(ogAny, "warn", "fail")), ifs(ogOK, "Title and image set", ifs(ogAny, "Incomplete", "Missing")), "Add og:title, og:description and og:image so links look good when shared.", 2))
	add(chk(S, "Twitter/X card", ifs(meta(doc, "twitter:card") != "", "pass", "warn"), ifs(meta(doc, "twitter:card") != "", "Present", "Missing"), "Add <meta name=\"twitter:card\" content=\"summary_large_image\">.", 1))
	imgs := reImg.FindAllString(doc, -1)
	withAlt := 0
	for _, t := range imgs {
		if strings.TrimSpace(attr(t, "alt")) != "" {
			withAlt++
		}
	}
	if len(imgs) == 0 {
		add(chk(S, "Image alt text", "pass", "No images found in HTML", "", 2))
	} else {
		cov := float64(withAlt) / float64(len(imgs)) * 100
		add(chk(S, "Image alt text", rate(cov, 90, 60), fmt.Sprintf("%.0f%% of %d images", cov, len(imgs)), "Describe every meaningful image in its alt attribute (helps SEO and accessibility).", 3))
	}
	rm := strings.ToLower(meta(doc, "robots"))
	noindex := strings.Contains(rm, "noindex")
	add(chk(S, "Indexable (no noindex)", ifs(noindex, "fail", "pass"), ifs(noindex, "Page is set to noindex", "Indexable"), "Remove the noindex directive if you want this page to appear in search.", 5))

	// Crawl & AI readiness
	hasRobots := okText(robots)
	add(chk(C, "robots.txt", ifs(hasRobots, "pass", "warn"), ifs(hasRobots, "Found", "Not found"), "Publish /robots.txt that allows crawlers and links your sitemap.", 2))
	sm := sitemap != nil && sitemap.status == 200 && (strings.Contains(string(sitemap.body), "<urlset") || strings.Contains(string(sitemap.body), "<sitemapindex"))
	if !sm && hasRobots && strings.Contains(strings.ToLower(string(robots.body)), "sitemap:") {
		sm = true
	}
	add(chk(C, "XML sitemap", ifs(sm, "pass", "fail"), ifs(sm, "Found", "Not found"), "Generate /sitemap.xml and submit it in Google Search Console and Bing Webmaster Tools.", 3))
	var blocked []string
	if hasRobots {
		blocked = blockedBots(string(robots.body))
	}
	add(chk(C, "AI crawlers allowed", ifs(len(blocked) == 0, "pass", "fail"), ifs(len(blocked) == 0, "Not blocked", "Blocked: "+strings.Join(blocked, ", ")), "Allow GPTBot, ClaudeBot, PerplexityBot and Google-Extended in robots.txt if you want AI assistants to cite you.", 4))
	ld := len(reLd.FindAllString(doc, -1))
	add(chk(C, "Structured data (JSON-LD)", ifs(ld > 0, "pass", "fail"), fmt.Sprintf("%d block(s)", ld), "Add schema.org JSON-LD (Organization, Product, FAQ, Article) so search and AI engines understand your content.", 4))
	hasLlms := okText(llms)
	add(chk(C, "llms.txt", ifs(hasLlms, "pass", "warn"), ifs(hasLlms, "Found", "Not found"), "Optional: publish /llms.txt summarising your site for AI assistants.", 2))

	// Performance (Google PageSpeed)
	if ps.OK {
		add(chk(P, "Performance score (mobile)", rate(float64(ps.Perf), 90, 50), fmt.Sprintf("%d/100", ps.Perf), "Compress images (WebP/AVIF), defer non-critical JavaScript, and use a CDN.", 5))
		add(chk(P, "Largest Contentful Paint", low(ps.LCP, 2500, 4000), fmt.Sprintf("%.1f s", ps.LCP/1000), "Optimise the hero image/text: preload it, compress it, and serve it from a CDN.", 5))
		add(chk(P, "Layout shift (CLS)", low(ps.CLS, 0.1, 0.25), fmt.Sprintf("%.2f", ps.CLS), "Set width/height on images and embeds; avoid inserting content above existing content.", 4))
		add(chk(P, "Total blocking time", low(ps.TBT, 200, 600), fmt.Sprintf("%.0f ms", ps.TBT), "Split or defer heavy JavaScript and remove unused third-party scripts.", 4))
		add(chk(S, "Lighthouse SEO score", rate(float64(ps.SEO), 90, 70), fmt.Sprintf("%d/100", ps.SEO), "Fix the SEO items Lighthouse flags (links, tap targets, crawlability).", 3))
		add(chk(A, "Accessibility score", rate(float64(ps.A11y), 90, 70), fmt.Sprintf("%d/100", ps.A11y), "Improve colour contrast, labels, and keyboard focus; fix issues in Lighthouse's accessibility report.", 3))
		add(chk(T, "Best-practices score", rate(float64(ps.BP), 90, 70), fmt.Sprintf("%d/100", ps.BP), "Fix console errors, use HTTPS everywhere and update outdated libraries.", 2))
		r.Charts = append(r.Charts, Chart{Title: "Google Lighthouse scores (mobile)", Type: "bar", Labels: []string{"Performance", "SEO", "Accessibility", "Best practices"}, Datasets: []Dataset{{Name: "Score", Data: []float64{float64(ps.Perf), float64(ps.SEO), float64(ps.A11y), float64(ps.BP)}}}})
	} else {
		r.Notes = append(r.Notes, "Google PageSpeed data was unavailable (its anonymous quota may be busy). Re-run in a minute, or add a free PSI_API_KEY in Render for reliable speed tests.")
	}

	// AI visibility
	if vis != nil {
		pct := float64(vis.Mentions) / float64(vis.Total) * 100
		add(chk("AI visibility", "Mention rate in AI answers", rate(pct, 60, 20), fmt.Sprintf("%d of %d buyer questions (%.0f%%)", vis.Mentions, vis.Total, pct), "Publish clear, quotable pages that answer these buyer questions, earn mentions on reputable sites, and add structured data and FAQs.", 5))
		if len(vis.Rivals) > 0 {
			ch := Chart{Title: "Brands AI recommends instead (times mentioned)", Type: "bar", Datasets: []Dataset{{Name: "Mentions"}}}
			for _, p := range vis.Rivals {
				ch.Labels = append(ch.Labels, p.Label)
				ch.Datasets[0].Data = append(ch.Datasets[0].Data, p.Value)
			}
			r.Charts = append(r.Charts, ch)
		}
		r.KPIs = append(r.KPIs, KPI{Label: "AI mention rate", Value: fmt.Sprintf("%.0f%%", pct), Note: "Category: " + vis.Category, Status: rate(pct, 60, 20)})
	} else if in.UseAI {
		r.Notes = append(r.Notes, "AI visibility test was skipped (free AI quota busy or unavailable).")
		_ = visErr
	}

	// KPIs
	byName := map[string]Check{}
	for _, c := range r.Checks {
		byName[c.Name] = c
	}
	pre := []KPI{}
	if c, ok := byName["Performance score (mobile)"]; ok {
		pre = append(pre, kpiFrom(c, "Mobile performance", "Google Lighthouse"))
		pre = append(pre, kpiFrom(byName["Largest Contentful Paint"], "LCP", "Good \u2264 2.5 s"))
		pre = append(pre, kpiFrom(byName["Layout shift (CLS)"], "CLS", "Good \u2264 0.1"))
	}
	pre = append(pre, kpiFrom(byName["Server response time"], "Server response", "Good \u2264 800 ms"))
	seoIssues := 0
	for _, c := range r.Checks {
		if c.Category == S && c.Status != "pass" {
			seoIssues++
		}
	}
	pre = append(pre, KPI{Label: "On-page SEO issues", Value: fmt.Sprint(seoIssues), Note: "Checks not passing", Status: ifs(seoIssues == 0, "pass", ifs(seoIssues <= 2, "warn", "fail"))})
	r.KPIs = append(pre, r.KPIs...)

	r.finish()
	if in.UseAI {
		extra := "Website title: " + title + ". Description: " + desc
		if vis != nil {
			extra += ". AI category: " + vis.Category
		}
		r.AI = s.insights(ctx, r, extra)
		if r.AI == nil {
			r.Notes = append(r.Notes, "AI written insights were unavailable this time. The measurements above are still accurate.")
		}
	}
	return r, nil
}
