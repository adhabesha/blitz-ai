package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
)

// LeadForge-style outbound: B2B prospect search, enrichment, staging ("All leads") and AI cold email drafts.

var apiClient = &http.Client{Timeout: 25 * time.Second}

func hid(parts ...string) string {
	h := sha1.Sum([]byte(strings.ToLower(strings.Join(parts, "|"))))
	return hex.EncodeToString(h[:])[:20]
}

var (
	reEmailFind = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9\-]+(?:\.[a-zA-Z0-9\-]+)*\.[a-zA-Z]{2,}`)
	rePhoneTel  = regexp.MustCompile(`tel:([+0-9 ()\-.]{7,22})`)
	rePhoneTxt  = regexp.MustCompile(`\+\d[\d\s().\-]{8,16}\d`)
	reLinkedIn  = regexp.MustCompile(`https?://(?:[a-z]{2,3}\.)?linkedin\.com/(?:in|company)/[A-Za-z0-9_%\-]+`)
)

var junkMail = []string{"example.", "sentry", "wixpress", "domain.com", "yourname", "your@", "email@", "user@", "name@", "noreply", "no-reply", "donotreply", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".css", ".js", "@2x"}
var genericLocal = map[string]bool{"info": true, "contact": true, "hello": true, "sales": true, "support": true, "office": true, "admin": true, "enquiries": true, "enquiry": true, "mail": true, "team": true, "hi": true}

func junkEmail(e string) bool {
	e = strings.ToLower(e)
	for _, j := range junkMail {
		if strings.Contains(e, j) {
			return true
		}
	}
	return false
}

var mxMu sync.Mutex
var mxCache = map[string]bool{}

// hasMX is a free check that a domain can receive email at all.
func hasMX(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false
	}
	mxMu.Lock()
	v, ok := mxCache[domain]
	mxMu.Unlock()
	if ok {
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	mx, err := net.DefaultResolver.LookupMX(ctx, domain)
	res := err == nil && len(mx) > 0
	mxMu.Lock()
	mxCache[domain] = res
	mxMu.Unlock()
	return res
}

type prospect struct {
	Name, Title, Company, Domain, Email, EmailStatus, Phone, LinkedIn, Location, Industry, Size, Source string
}

func (p prospect) id() string {
	if p.Email != "" {
		return hid(p.Email)
	}
	return hid(p.Name, p.Domain, p.Company)
}

func (p prospect) doc(by string) map[string]any {
	return map[string]any{"name": clip(p.Name, 120), "title": clip(p.Title, 120), "company": clip(p.Company, 120), "domain": clip(p.Domain, 120),
		"email": strings.ToLower(clip(p.Email, 160)), "emailStatus": p.EmailStatus, "phone": clip(p.Phone, 40), "linkedin": clip(p.LinkedIn, 200),
		"location": clip(p.Location, 120), "industry": clip(p.Industry, 120), "size": clip(p.Size, 40), "source": p.Source,
		"promoted": false, "createdAt": time.Now(), "createdBy": by}
}

func cleanDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
	d = strings.TrimPrefix(d, "www.")
	if i := strings.IndexAny(d, "/?#"); i >= 0 {
		d = d[:i]
	}
	if !strings.Contains(d, ".") || strings.ContainsAny(d, " @") {
		return ""
	}
	return d
}

// ---------------------------------------------------------------- settings

type obSettings struct {
	Offer, FromName, Company, Address, ReplyTo string
	Tz                                         int
}

func obOf(ws map[string]any) obSettings {
	m, _ := ws["outbound"].(map[string]any)
	return obSettings{Offer: sv(m, "offer"), FromName: sv(m, "fromName"), Company: sv(m, "company"), Address: firstNonEmptyStr(sv(m, "address"), env("COMPANY_ADDRESS")),
		ReplyTo: firstNonEmptyStr(sv(m, "replyTo"), env("REPLY_TO")), Tz: int(nv(m, "tz"))}
}

func (s *server) crmObSettings(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'v') {
		c.deny(w, "view outbound")
		return
	}
	if r.Method == http.MethodPost {
		if !c.can("integrations", 'e') {
			c.deny(w, "change outbound settings")
			return
		}
		var in struct {
			Offer, FromName, Company, Address, ReplyTo string
			Tz                                         int
		}
		if !readBody(w, r, &in, 20<<10) {
			return
		}
		if in.ReplyTo != "" && !reEmailOK.MatchString(in.ReplyTo) {
			http.Error(w, "The reply-to address is not valid.", 400)
			return
		}
		if in.Tz < -12 || in.Tz > 14 {
			in.Tz = 0
		}
		_, err := s.crmWS(c.wid).Set(c.ctx, map[string]any{"outbound": map[string]any{"offer": ctext(in.Offer, 1500), "fromName": clip(in.FromName, 80), "company": clip(in.Company, 120), "address": clip(in.Address, 200), "replyTo": clip(in.ReplyTo, 160), "tz": in.Tz}}, firestoreMerge())
		if err != nil {
			http.Error(w, "Could not save.", 500)
			return
		}
		jsonOut(w, map[string]bool{"ok": true})
		return
	}
	secret := sv(c.ws, "inboundSecret")
	if secret == "" {
		secret = randHex(12)
		s.crmWS(c.wid).Set(c.ctx, map[string]any{"inboundSecret": secret}, firestoreMerge())
	}
	o := obOf(c.ws)
	jsonOut(w, map[string]any{
		"offer": o.Offer, "fromName": o.FromName, "company": o.Company, "address": o.Address, "replyTo": o.ReplyTo, "tz": o.Tz,
		"inboundUrl": baseURL(r) + "/api/crm/inbound/" + c.wid + "/" + secret,
		"mailReady":  mailConfigured(), "smtp": smtpReady(), "baseUrl": env("BASE_URL") != "",
		"apollo": env("APOLLO_API_KEY") != "", "hunter": env("HUNTER_API_KEY") != "", "ai": env("GEMINI_API_KEY") != "",
	})
}

// ---------------------------------------------------------------- providers

func httpJSON(ctx context.Context, method, u string, hdr map[string]string, body any, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := apiClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 300 && out != nil {
		if e := json.Unmarshal(b, out); e != nil {
			return resp.StatusCode, e
		}
	}
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("status %d: %s", resp.StatusCode, trunc(strings.ReplaceAll(string(b), "\n", " "), 160))
	}
	return resp.StatusCode, nil
}

type prospectQ struct {
	Titles   []string `json:"titles"`
	Industry string   `json:"industry"`
	Size     string   `json:"size"`
	Location string   `json:"location"`
	Keywords string   `json:"keywords"`
	Count    int      `json:"count"`
	Mode     string   `json:"mode"`
}

func apolloRange(sz string) []string {
	sz = strings.ReplaceAll(strings.TrimSpace(sz), " ", "")
	switch {
	case sz == "":
		return nil
	case strings.HasSuffix(sz, "+"):
		return []string{strings.TrimSuffix(sz, "+") + ",100000"}
	case strings.Contains(sz, "-"):
		return []string{strings.Replace(sz, "-", ",", 1)}
	}
	return nil
}

func (s *server) apolloSearch(ctx context.Context, q prospectQ) ([]prospect, error) {
	key := env("APOLLO_API_KEY")
	hd := map[string]string{"X-Api-Key": key}
	body := map[string]any{"per_page": q.Count, "page": 1}
	if len(q.Titles) > 0 {
		body["person_titles"] = q.Titles
	}
	if q.Location != "" {
		body["person_locations"] = []string{q.Location}
	}
	if r := apolloRange(q.Size); r != nil {
		body["organization_num_employees_ranges"] = r
	}
	if kw := strings.TrimSpace(q.Industry + " " + q.Keywords); kw != "" {
		body["q_keywords"] = kw
	}
	var res struct {
		People []struct {
			ID, Title string
			FirstName string `json:"first_name"`
		} `json:"people"`
	}
	_, err := httpJSON(ctx, "POST", "https://api.apollo.io/api/v1/mixed_people/api_search", hd, body, &res)
	if err != nil {
		res.People = nil
		if _, err2 := httpJSON(ctx, "POST", "https://api.apollo.io/api/v1/mixed_people/search", hd, body, &res); err2 != nil {
			if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") {
				return nil, errors.New("Apollo refused the key. Check APOLLO_API_KEY in Admin; API search needs a paid Apollo plan and a master API key. (" + trunc(err.Error(), 100) + ")")
			}
			return nil, errors.New("Apollo search failed: " + trunc(err.Error(), 140))
		}
	}
	type enr struct {
		Person struct {
			First        string `json:"first_name"`
			Last         string `json:"last_name"`
			Name         string `json:"name"`
			Email        string `json:"email"`
			EmailStatus  string `json:"email_status"`
			Title        string `json:"title"`
			Linkedin     string `json:"linkedin_url"`
			City         string `json:"city"`
			State        string `json:"state"`
			Country      string `json:"country"`
			Phones       []struct{ SanitizedNumber string `json:"sanitized_number"` } `json:"phone_numbers"`
			Organization struct {
				Name     string  `json:"name"`
				Website  string  `json:"website_url"`
				Domain   string  `json:"primary_domain"`
				Industry string  `json:"industry"`
				Employee float64 `json:"estimated_num_employees"`
			} `json:"organization"`
		} `json:"person"`
	}
	out := make([]prospect, len(res.People))
	okc := make([]bool, len(res.People))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, p := range res.People {
		if i >= q.Count {
			break
		}
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var e enr
			if _, err := httpJSON(ctx, "POST", "https://api.apollo.io/api/v1/people/match", hd, map[string]any{"id": id, "reveal_personal_emails": false}, &e); err != nil {
				return
			}
			p := e.Person
			pr := prospect{Name: firstNonEmptyStr(p.Name, strings.TrimSpace(p.First+" "+p.Last)), Title: p.Title, Company: p.Organization.Name,
				Domain: cleanDomain(firstNonEmptyStr(p.Organization.Domain, p.Organization.Website)), LinkedIn: p.Linkedin, Industry: p.Organization.Industry, Source: "apollo",
				Location: strings.Trim(strings.Join([]string{p.City, p.State, p.Country}, ", "), ", ")}
			if p.Organization.Employee > 0 {
				pr.Size = fmt.Sprintf("%d", int(p.Organization.Employee))
			}
			if strings.Contains(p.Email, "@") && !strings.Contains(p.Email, "not_unlocked") {
				pr.Email = p.Email
				pr.EmailStatus = ifs(p.EmailStatus == "verified", "verified", firstNonEmptyStr(p.EmailStatus, "unverified"))
			}
			if len(p.Phones) > 0 {
				pr.Phone = p.Phones[0].SanitizedNumber
			}
			out[i], okc[i] = pr, pr.Name != ""
		}(i, p.ID)
	}
	wg.Wait()
	var rows []prospect
	for i, p := range out {
		if okc[i] {
			rows = append(rows, p)
		}
	}
	return rows, nil
}

func (s *server) hunterDomain(ctx context.Context, domain string, titles []string, limit int) ([]prospect, string, error) {
	var res struct {
		Data struct {
			Organization string `json:"organization"`
			Emails       []struct {
				Value      string `json:"value"`
				First      string `json:"first_name"`
				Last       string `json:"last_name"`
				Position   string `json:"position"`
				Confidence int    `json:"confidence"`
				Phone      string `json:"phone_number"`
				Linkedin   string `json:"linkedin"`
				Ver        struct {
					Status string `json:"status"`
				} `json:"verification"`
			} `json:"emails"`
		} `json:"data"`
	}
	u := "https://api.hunter.io/v2/domain-search?domain=" + url.QueryEscape(domain) + "&limit=10&api_key=" + url.QueryEscape(env("HUNTER_API_KEY"))
	if _, err := httpJSON(ctx, "GET", u, nil, nil, &res); err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "429") {
			return nil, "", errors.New("Hunter refused the request (key invalid or monthly credits used up)")
		}
		return nil, "", err
	}
	type sc struct {
		p prospect
		n int
	}
	var all []sc
	for _, e := range res.Data.Emails {
		pr := prospect{Name: strings.TrimSpace(e.First + " " + e.Last), Title: e.Position, Company: res.Data.Organization, Domain: domain, Email: e.Value, Phone: e.Phone, LinkedIn: e.Linkedin, Source: "hunter",
			EmailStatus: firstNonEmptyStr(ifs(e.Ver.Status == "valid", "verified", e.Ver.Status), fmt.Sprintf("confidence %d%%", e.Confidence))}
		n := e.Confidence
		low := strings.ToLower(e.Position)
		for _, t := range titles {
			for _, w := range strings.Fields(strings.ToLower(t)) {
				if len(w) > 2 && strings.Contains(low, w) {
					n += 100
				}
			}
		}
		if pr.Name == "" {
			n -= 50
			pr.Name = "Team inbox"
		}
		all = append(all, sc{pr, n})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].n > all[j].n })
	var out []prospect
	for i, x := range all {
		if i >= limit {
			break
		}
		out = append(out, x.p)
	}
	return out, res.Data.Organization, nil
}

// groundedJSON asks Gemini with live Google Search, trying several models, then without search.
func (s *server) groundedJSON(ctx context.Context, prompt string) (string, bool, error) {
	if env("GEMINI_API_KEY") == "" {
		return "", false, errors.New("AI is not set up (GEMINI_API_KEY)")
	}
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
	cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	for _, m := range models {
		if txt, err := s.geminiSearch(cctx, m, prompt); err == nil {
			return txt, true, nil
		} else {
			log.Println("grounded search with", m, "failed:", err)
		}
		if cctx.Err() != nil {
			break
		}
	}
	cc, c2 := context.WithTimeout(ctx, 30*time.Second)
	defer c2()
	txt, err := s.gemini(cc, strings.Replace(prompt, "Use Google Search. ", "", 1))
	if err != nil {
		if errors.Is(err, errRate) {
			return "", false, errors.New("The AI is busy right now (limit reached). Wait a minute and try again.")
		}
		return "", false, err
	}
	return txt, false, nil
}

type webCo struct {
	Name, Domain, Industry, Size, Location string
	People                                 []struct{ Name, Title, Linkedin string }
}

func (s *server) aiCompanies(ctx context.Context, q prospectQ) ([]webCo, bool, error) {
	n := q.Count
	if n > 12 {
		n = 12
	}
	prompt := fmt.Sprintf(`Use Google Search. Find %d real companies that match this ideal customer profile.
Industry: %s
Company size (employees, may be empty): %s
Location: %s
Extra keywords: %s
Decision makers wanted (job titles): %s

For each company give its official website domain and up to 2 decision makers whose name and title are publicly listed (company website, public LinkedIn page, press or news). Never invent a person or a domain: if you did not see a person, leave "people" empty. Only include companies that have their own website.
Return JSON only: {"companies":[{"name":"","domain":"example.com","industry":"","size":"","location":"","people":[{"name":"","title":"","linkedin":""}]}]}`,
		n, clip(q.Industry, 100), clip(q.Size, 30), clip(q.Location, 100), clip(q.Keywords, 150), strings.Join(q.Titles, ", "))
	txt, live, err := s.groundedJSON(ctx, prompt)
	if err != nil {
		return nil, false, err
	}
	var res struct {
		Companies []struct {
			Name     string `json:"name"`
			Domain   string `json:"domain"`
			Industry string `json:"industry"`
			Size     string `json:"size"`
			Location string `json:"location"`
			People   []struct {
				Name     string `json:"name"`
				Title    string `json:"title"`
				Linkedin string `json:"linkedin"`
			} `json:"people"`
		} `json:"companies"`
	}
	if err := json.Unmarshal([]byte(cleanJSON(txt)), &res); err != nil {
		return nil, live, errors.New("The AI answer could not be read. Try again.")
	}
	var out []webCo
	seen := map[string]bool{}
	for _, c := range res.Companies {
		d := cleanDomain(c.Domain)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		wc := webCo{Name: c.Name, Domain: d, Industry: c.Industry, Size: c.Size, Location: c.Location}
		for _, p := range c.People {
			if strings.TrimSpace(p.Name) != "" {
				wc.People = append(wc.People, struct{ Name, Title, Linkedin string }{p.Name, p.Title, p.Linkedin})
			}
		}
		out = append(out, wc)
	}
	return out, live, nil
}

type siteInfo struct {
	emails   []string
	phone    string
	linkedin string
	about    string
}

// crawlSite reads the public contact pages of a company site for emails, phones and LinkedIn links.
func crawlSite(ctx context.Context, domain string) siteInfo {
	var si siteInfo
	seen := map[string]bool{}
	for _, p := range []string{"", "/contact", "/contact-us", "/about", "/about-us", "/team"} {
		if ctx.Err() != nil {
			break
		}
		pc, cancel := context.WithTimeout(ctx, 9*time.Second)
		f, err := fetch(pc, "https://"+domain+p, 600<<10)
		cancel()
		if err != nil || f.status >= 400 {
			continue
		}
		doc := html.UnescapeString(string(f.body))
		doc = strings.NewReplacer(" [at] ", "@", "[at]", "@", "(at)", "@", " (at) ", "@", "&#64;", "@").Replace(doc)
		if p == "" {
			si.about = firstNonEmptyStr(meta(doc, "description"), "")
		}
		for _, e := range reEmailFind.FindAllString(doc, 40) {
			e = strings.ToLower(strings.Trim(e, ".,;:"))
			if !junkEmail(e) && !seen[e] {
				seen[e] = true
				si.emails = append(si.emails, e)
			}
		}
		if si.phone == "" {
			if m := rePhoneTel.FindStringSubmatch(doc); m != nil {
				si.phone = strings.TrimSpace(m[1])
			} else if m := rePhoneTxt.FindString(pageText(doc, 20000)); m != "" {
				si.phone = strings.TrimSpace(m)
			}
		}
		if si.linkedin == "" {
			si.linkedin = reLinkedIn.FindString(doc)
		}
		if len(si.emails) >= 4 && si.phone != "" && p != "" {
			break
		}
	}
	// prefer addresses on the company's own domain
	sort.SliceStable(si.emails, func(i, j int) bool {
		return strings.HasSuffix(si.emails[i], "@"+domain) && !strings.HasSuffix(si.emails[j], "@"+domain)
	})
	return si
}

func personalMatch(name string, emails []string) string {
	parts := strings.Fields(strings.ToLower(name))
	if len(parts) == 0 {
		return ""
	}
	f, l := parts[0], parts[len(parts)-1]
	for _, e := range emails {
		lp := e[:strings.Index(e, "@")]
		if strings.Contains(lp, f) || (len(parts) > 1 && (strings.Contains(lp, l) || lp == f[:1]+l)) {
			return e
		}
	}
	return ""
}

func (s *server) webProspects(ctx context.Context, q prospectQ, notes *[]string) ([]prospect, error) {
	cos, live, err := s.aiCompanies(ctx, q)
	if err != nil {
		return nil, err
	}
	if !live {
		*notes = append(*notes, "Live web search was not available, so the AI used its own knowledge; every website is still opened and checked.")
	}
	useHunter := env("HUNTER_API_KEY") != ""
	rows := make([][]prospect, len(cos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	for i, co := range cos {
		wg.Add(1)
		go func(i int, co webCo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			si := crawlSite(cctx, co.Domain)
			base := prospect{Company: co.Name, Domain: co.Domain, Industry: co.Industry, Size: co.Size, Location: co.Location, Phone: si.phone, Source: "ai_web"}
			var list []prospect
			if useHunter {
				if hp, org, e := s.hunterDomain(cctx, co.Domain, q.Titles, 3); e == nil {
					for _, p := range hp {
						p.Industry, p.Size, p.Location = base.Industry, base.Size, base.Location
						p.Company = firstNonEmptyStr(p.Company, org, co.Name)
						p.Phone = firstNonEmptyStr(p.Phone, si.phone)
						p.Source = "ai_web+hunter"
						list = append(list, p)
					}
				}
			}
			if len(list) == 0 {
				used := map[string]bool{}
				for _, pe := range co.People {
					p := base
					p.Name, p.Title, p.LinkedIn = pe.Name, pe.Title, pe.Linkedin
					if m := personalMatch(pe.Name, si.emails); m != "" {
						p.Email, p.EmailStatus = m, "found on website"
						used[m] = true
					}
					list = append(list, p)
				}
				var gen string
				for _, e := range si.emails {
					if !used[e] && (gen == "" || genericLocal[e[:strings.Index(e, "@")]]) && strings.HasSuffix(e, "@"+co.Domain) {
						gen = e
					}
				}
				if gen == "" && len(si.emails) > 0 && !used[si.emails[0]] {
					gen = si.emails[0]
				}
				if len(list) == 0 || (gen != "" && len(list) < 3) {
					p := base
					p.Name, p.LinkedIn = "Team inbox ("+co.Name+")", si.linkedin
					if gen != "" {
						p.Email, p.EmailStatus = gen, "found on website (general inbox)"
					}
					if len(list) == 0 || gen != "" {
						list = append(list, p)
					}
				}
			}
			for k := range list {
				if list[k].Email != "" && !hasMX(co.Domain) {
					list[k].EmailStatus = "domain cannot receive mail"
				}
			}
			rows[i] = list
		}(i, co)
	}
	wg.Wait()
	var out []prospect
	for _, r := range rows {
		out = append(out, r...)
	}
	return out, nil
}

// ---------------------------------------------------------------- handlers

func (s *server) crmProspectSearch(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'c') {
		c.deny(w, "search for prospects")
		return
	}
	var q prospectQ
	if !readBody(w, r, &q, 20<<10) {
		return
	}
	if q.Count < 1 {
		q.Count = 10
	}
	if q.Count > 15 {
		q.Count = 15
	}
	var tl []string
	for _, t := range q.Titles {
		if t = clip(t, 60); t != "" && len(tl) < 8 {
			tl = append(tl, t)
		}
	}
	q.Titles = tl
	q.Industry, q.Location, q.Keywords, q.Size = clip(q.Industry, 100), clip(q.Location, 100), clip(q.Keywords, 150), clip(q.Size, 30)
	if q.Industry == "" && q.Keywords == "" && len(q.Titles) == 0 {
		http.Error(w, "Describe who you are looking for: an industry, keywords or job titles.", 400)
		return
	}
	col := s.crmWS(c.wid).Collection("prospects")
	{
		it := col.Select().Limit(501).Documents(c.ctx)
		n := 0
		for {
			if _, err := it.Next(); err != nil {
				break
			}
			n++
		}
		it.Stop()
		if n >= 500 {
			http.Error(w, "You have 500 prospects waiting. Move the good ones to My Leads and delete the rest, then search again.", 400)
			return
		}
	}
	mode := q.Mode
	if mode == "" || mode == "auto" {
		mode = ifs(env("APOLLO_API_KEY") != "", "apollo", "web")
	}
	ctx, cancel := context.WithTimeout(c.ctx, 110*time.Second)
	defer cancel()
	notes := []string{}
	var rows []prospect
	var err error
	switch mode {
	case "apollo":
		if env("APOLLO_API_KEY") == "" {
			http.Error(w, "Add APOLLO_API_KEY in Admin first, or use the free AI web search.", 400)
			return
		}
		rows, err = s.apolloSearch(ctx, q)
	default:
		mode = "web"
		rows, err = s.webProspects(ctx, q, &notes)
		if env("HUNTER_API_KEY") == "" {
			notes = append(notes, "Emails come from each company's public website. Add a Hunter key in Admin to find more named, verified emails.")
		}
	}
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	added, dup, noMail := 0, 0, 0
	for _, p := range rows {
		if p.Email != "" {
			p.Email = strings.ToLower(p.Email)
			if p.EmailStatus == "" {
				p.EmailStatus = "unverified"
			}
		} else {
			noMail++
			p.EmailStatus = "no email found"
		}
		ref := col.Doc(p.id())
		if _, e := ref.Create(c.ctx, p.doc(c.uid)); e != nil {
			dup++
		} else {
			added++
		}
	}
	if noMail > 0 {
		notes = append(notes, fmt.Sprintf("%d people have no email yet. Use Find email on them (needs the company website) or add a Hunter or Apollo key.", noMail))
	}
	jsonOut(w, map[string]any{"added": added, "duplicates": dup, "found": len(rows), "mode": mode, "notes": notes})
}

func (s *server) crmProspects(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'v') {
		c.deny(w, "view prospects")
		return
	}
	it := s.crmWS(c.wid).Collection("prospects").OrderBy("createdAt", firestore.Desc).Limit(500).Documents(c.ctx)
	defer it.Stop()
	out := []map[string]any{}
	for {
		d, err := it.Next()
		if err == iterator.Done || err != nil {
			break
		}
		x := d.Data()
		x["id"] = d.Ref.ID
		out = append(out, x)
	}
	jsonOut(w, map[string]any{"items": out})
}

var guessPats = []string{"{f}.{l}", "{f}", "{f}{l}", "{fi}{l}", "{f}{li}", "{f}_{l}"}

func (s *server) crmProspectLookup(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'c') {
		c.deny(w, "look up contacts")
		return
	}
	var in struct {
		Name, Domain, Title, Company string
		Save                         bool
	}
	if !readBody(w, r, &in, 10<<10) {
		return
	}
	dom := cleanDomain(in.Domain)
	parts := strings.Fields(strings.ToLower(in.Name))
	if dom == "" || len(parts) == 0 {
		http.Error(w, "Enter the person's full name and the company website.", 400)
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 40*time.Second)
	defer cancel()
	type cand struct {
		Email  string `json:"email"`
		Status string `json:"status"`
	}
	var cands []cand
	note := ""
	if env("HUNTER_API_KEY") != "" {
		var res struct {
			Data struct {
				Email string `json:"email"`
				Score int    `json:"score"`
				Ver   struct {
					Status string `json:"status"`
				} `json:"verification"`
			} `json:"data"`
		}
		u := "https://api.hunter.io/v2/email-finder?domain=" + url.QueryEscape(dom) + "&full_name=" + url.QueryEscape(in.Name) + "&api_key=" + url.QueryEscape(env("HUNTER_API_KEY"))
		if _, err := httpJSON(ctx, "GET", u, nil, nil, &res); err == nil && res.Data.Email != "" {
			cands = append(cands, cand{res.Data.Email, firstNonEmptyStr(ifs(res.Data.Ver.Status == "valid", "verified", res.Data.Ver.Status), fmt.Sprintf("confidence %d%%", res.Data.Score))})
		} else if err != nil {
			note = "Hunter could not answer (" + trunc(err.Error(), 80) + "). Showing pattern guesses."
		}
	}
	if len(cands) == 0 {
		f, l := parts[0], parts[len(parts)-1]
		if len(parts) == 1 {
			l = ""
		}
		mx := hasMX(dom)
		seen := map[string]bool{}
		for _, p := range guessPats {
			if l == "" && strings.Contains(p, "{l}") {
				continue
			}
			e := strings.NewReplacer("{f}", f, "{l}", l, "{fi}", f[:1], "{li}", ifs(l != "", l[:1], "")).Replace(p) + "@" + dom
			if !seen[e] && reEmailOK.MatchString(e) {
				seen[e] = true
				cands = append(cands, cand{e, ifs(mx, "guess (domain accepts mail)", "guess (domain has no mail server)")})
			}
		}
		if note == "" {
			note = "These are pattern guesses, not confirmed addresses. Add a Hunter key to get verified ones."
		}
	}
	saved := false
	if in.Save && len(cands) > 0 {
		p := prospect{Name: clip(in.Name, 120), Title: in.Title, Company: firstNonEmptyStr(in.Company, dom), Domain: dom, Email: cands[0].Email, EmailStatus: cands[0].Status, Source: "lookup"}
		if _, err := s.crmWS(c.wid).Collection("prospects").Doc(p.id()).Create(c.ctx, p.doc(c.uid)); err == nil {
			saved = true
		}
	}
	jsonOut(w, map[string]any{"candidates": cands, "note": note, "saved": saved})
}

func (s *server) crmProspectPromote(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'c') {
		c.deny(w, "add leads")
		return
	}
	var in struct {
		IDs      []string `json:"ids"`
		OwnerUID string   `json:"ownerUid"`
	}
	if !readBody(w, r, &in, 50<<10) {
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > 200 {
		http.Error(w, "Pick between 1 and 200 prospects.", 400)
		return
	}
	if !c.validOwner(in.OwnerUID) {
		http.Error(w, "That owner is not a member of this workspace.", 400)
		return
	}
	owner := in.OwnerUID
	if owner == "" {
		owner = c.uid
	}
	col := s.crmWS(c.wid).Collection("prospects")
	created, dup := 0, 0
	leadIDs := []string{}
	for _, id := range in.IDs {
		d, err := col.Doc(id).Get(c.ctx)
		if err != nil {
			continue
		}
		x := d.Data()
		f := map[string]any{"name": firstNonEmptyStr(sv(x, "name"), sv(x, "email")), "company": sv(x, "company"), "email": sv(x, "email"), "phone": sv(x, "phone"),
			"title": sv(x, "title"), "source": "outbound", "ownerUid": owner, "tags": []string{"outbound"}, "country": "", "city": sv(x, "location")}
		notes := []string{}
		for _, k := range [][2]string{{"domain", "Website"}, {"linkedin", "LinkedIn"}, {"industry", "Industry"}, {"size", "Size"}, {"emailStatus", "Email status"}} {
			if v := sv(x, k[0]); v != "" {
				notes = append(notes, k[1]+": "+v)
			}
		}
		f["notes"] = strings.Join(notes, "\n")
		lid, isDup, err := s.crmAddLead(c.ctx, c.wid, f, c.uid, "Prospected by outbound search")
		if err != nil {
			continue
		}
		if isDup {
			dup++
		} else {
			created++
		}
		leadIDs = append(leadIDs, lid)
		col.Doc(id).Set(c.ctx, map[string]any{"promoted": true, "leadId": lid}, firestoreMerge())
	}
	jsonOut(w, map[string]any{"created": created, "duplicates": dup, "leadIds": leadIDs})
}

func (s *server) crmProspectDelete(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'e') {
		c.deny(w, "delete prospects")
		return
	}
	var in struct {
		IDs []string `json:"ids"`
	}
	if !readBody(w, r, &in, 50<<10) {
		return
	}
	n := 0
	for i, id := range in.IDs {
		if i >= 500 || id == "" || strings.Contains(id, "/") {
			continue
		}
		if _, err := s.crmWS(c.wid).Collection("prospects").Doc(id).Delete(c.ctx); err == nil {
			n++
		}
	}
	jsonOut(w, map[string]int{"deleted": n})
}

func (s *server) crmProspectEdit(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'e') {
		c.deny(w, "edit prospects")
		return
	}
	var in struct {
		ID, Name, Title, Company, Email, Phone string
	}
	if !readBody(w, r, &in, 10<<10) {
		return
	}
	if in.ID == "" || strings.Contains(in.ID, "/") {
		http.Error(w, "bad request", 400)
		return
	}
	em := strings.ToLower(strings.TrimSpace(in.Email))
	if em != "" && !reEmailOK.MatchString(em) {
		http.Error(w, "That email address is not valid.", 400)
		return
	}
	up := map[string]any{"name": clip(in.Name, 120), "title": clip(in.Title, 120), "company": clip(in.Company, 120), "email": em, "phone": clip(in.Phone, 40)}
	if em != "" {
		up["emailStatus"] = "entered by hand"
	}
	if _, err := s.crmWS(c.wid).Collection("prospects").Doc(in.ID).Set(c.ctx, up, firestoreMerge()); err != nil {
		http.Error(w, "Could not save.", 500)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

// crmFindEmail tries to find a missing email for a staged prospect by reading its company website.
func (s *server) crmProspectFind(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'e') {
		c.deny(w, "edit prospects")
		return
	}
	var in struct{ ID string }
	if !readBody(w, r, &in, 2<<10) || in.ID == "" || strings.Contains(in.ID, "/") {
		return
	}
	ref := s.crmWS(c.wid).Collection("prospects").Doc(in.ID)
	d, err := ref.Get(c.ctx)
	if err != nil {
		http.Error(w, "Not found", 404)
		return
	}
	x := d.Data()
	dom := cleanDomain(sv(x, "domain"))
	if dom == "" {
		http.Error(w, "This prospect has no company website to search.", 400)
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 45*time.Second)
	defer cancel()
	email, status := "", ""
	if env("HUNTER_API_KEY") != "" && sv(x, "name") != "" && !strings.HasPrefix(sv(x, "name"), "Team inbox") {
		var res struct {
			Data struct {
				Email string `json:"email"`
				Score int    `json:"score"`
			} `json:"data"`
		}
		u := "https://api.hunter.io/v2/email-finder?domain=" + url.QueryEscape(dom) + "&full_name=" + url.QueryEscape(sv(x, "name")) + "&api_key=" + url.QueryEscape(env("HUNTER_API_KEY"))
		if _, e := httpJSON(ctx, "GET", u, nil, nil, &res); e == nil && res.Data.Email != "" {
			email, status = res.Data.Email, fmt.Sprintf("confidence %d%%", res.Data.Score)
		}
	}
	if email == "" {
		si := crawlSite(ctx, dom)
		if m := personalMatch(sv(x, "name"), si.emails); m != "" {
			email, status = m, "found on website"
		} else if len(si.emails) > 0 {
			email, status = si.emails[0], "found on website (general inbox)"
		}
		if si.phone != "" && sv(x, "phone") == "" {
			ref.Set(c.ctx, map[string]any{"phone": si.phone}, firestoreMerge())
		}
	}
	if email == "" {
		jsonOut(w, map[string]any{"found": false})
		return
	}
	ref.Set(c.ctx, map[string]any{"email": email, "emailStatus": status}, firestoreMerge())
	jsonOut(w, map[string]any{"found": true, "email": email, "status": status})
}

// ---------------------------------------------------------------- AI cold email

type draftOut struct {
	Subject   string   `json:"subject"`
	Body      string   `json:"body"`
	Followups []string `json:"followups"`
}

func (s *server) crmOutreachDraft(w http.ResponseWriter, r *http.Request) {
	c := s.crmNeed(w, r)
	if c == nil {
		return
	}
	if !c.can("leads", 'v') {
		c.deny(w, "draft emails")
		return
	}
	var in struct {
		LeadID, Name, Title, Company, Domain, Notes, Offer, Tone string
		Template                                                 bool
	}
	if !readBody(w, r, &in, 20<<10) {
		return
	}
	if env("GEMINI_API_KEY") == "" {
		http.Error(w, "AI is not set up (GEMINI_API_KEY).", 400)
		return
	}
	if in.LeadID != "" {
		if d, err := s.crmWS(c.wid).Collection("leads").Doc(in.LeadID).Get(c.ctx); err == nil && c.sees("leads", d.Data()) {
			x := d.Data()
			in.Name, in.Title, in.Company, in.Notes = firstNonEmptyStr(in.Name, sv(x, "name")), firstNonEmptyStr(in.Title, sv(x, "title")), firstNonEmptyStr(in.Company, sv(x, "company")), firstNonEmptyStr(in.Notes, sv(x, "notes"))
		}
	}
	o := obOf(c.ws)
	offer := firstNonEmptyStr(clip(in.Offer, 800), o.Offer)
	if offer == "" {
		http.Error(w, "Describe what you sell first (Outbound > Settings > What you offer).", 400)
		return
	}
	site := ""
	if d := cleanDomain(in.Domain); d != "" && !in.Template {
		pc, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		if f, err := fetch(pc, "https://"+d, 400<<10); err == nil && f.status < 400 {
			site = trunc(meta(string(f.body), "description")+" "+pageText(string(f.body), 500), 700)
		}
		cancel()
	}
	sender := firstNonEmptyStr(o.FromName, "the sender")
	who := fmt.Sprintf("Recipient: %s, %s at %s. Notes: %s. Their website says: %s", clip(in.Name, 80), clip(in.Title, 80), clip(in.Company, 80), ctext(in.Notes, 500), site)
	if in.Template {
		who = "Write a reusable TEMPLATE. Use the placeholders {{first_name}}, {{company}} and {{title}} where the recipient's details belong, and sign as {{sender}}. Do not invent facts about the recipient."
	}
	prompt := fmt.Sprintf(`You write high-performing B2B cold emails. Sender: %s from %s. What they sell: %s
%s
Tone: %s.
Rules: first email under 90 words, plain text, one specific opening line that shows you looked at them (only use facts given above; otherwise stay general, never invent), one clear benefit, one low-friction question as the call to action, no hype, no emojis, no attachments, no "I hope this finds you well". Subject under 7 words, lowercase style, no clickbait. Then write 2 short follow-ups (under 60 words each) that add new value, not "just checking in". Do not add a signature block or unsubscribe text; sign with the sender name only.
Return JSON only: {"subject":"","body":"","followups":["",""]}`, sender, firstNonEmptyStr(o.Company, "their company"), offer, who, firstNonEmptyStr(clip(in.Tone, 40), "friendly and direct"))
	ctx, cancel := context.WithTimeout(c.ctx, 40*time.Second)
	defer cancel()
	txt, err := s.gemini(ctx, prompt)
	if err != nil {
		http.Error(w, "The AI could not write the email: "+trunc(err.Error(), 100), 502)
		return
	}
	var d draftOut
	if json.Unmarshal([]byte(cleanJSON(txt)), &d) != nil || d.Body == "" {
		http.Error(w, "The AI answer could not be read. Try again.", 502)
		return
	}
	if d.Followups == nil {
		d.Followups = []string{}
	}
	jsonOut(w, d)
}
