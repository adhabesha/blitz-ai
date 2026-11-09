package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const gv = "https://graph.facebook.com/v25.0/"

var reAct = regexp.MustCompile(`^\d{5,20}$`)

func zeroDecimal(cur string) bool {
	switch strings.ToUpper(cur) {
	case "JPY", "KRW", "VND", "CLP", "ISK", "UGX", "XAF", "XOF", "PYG", "RWF":
		return true
	}
	return false
}

func minorUnits(v float64, cur string) int64 {
	if zeroDecimal(cur) {
		return int64(math.Round(v))
	}
	return int64(math.Round(v * 100))
}

func maxDaily() float64 {
	if v, err := strconv.ParseFloat(env("ADS_MAX_DAILY_BUDGET"), 64); err == nil && v > 0 {
		return v
	}
	return 20
}

type adAcct struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   int    `json:"account_status"`
	Currency string `json:"currency"`
	Spent    string `json:"amount_spent"`
}

func (s *server) adsAccounts(ctx context.Context, uid string) ([]adAcct, string, error) {
	var lastErr error
	for _, p := range []string{"instagram", "facebook"} {
		t, err := s.validToken(ctx, uid, provOf(p))
		if err != nil {
			lastErr = err
			continue
		}
		var d struct {
			Data []adAcct `json:"data"`
		}
		if err := getJSON(ctx, gv+"me/adaccounts?"+url.Values{"fields": {"id,name,account_status,currency,amount_spent"}, "limit": {"50"}, "access_token": {t.Access}}.Encode(), &d); err != nil {
			lastErr = err
			continue
		}
		if len(d.Data) > 0 {
			return d.Data, t.Access, nil
		}
	}
	if lastErr != nil {
		return nil, "", userErr("Could not read ad accounts: " + trunc(lastErr.Error(), 220))
	}
	return nil, "", userErr("No ad account found. In Meta Business settings create an ad account with a payment method, assign it to your Blitz AI app, then reconnect Instagram.")
}

func (s *server) adsAccountsHandler(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	accts, _, err := s.adsAccounts(ctx, uid)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"accounts": accts, "maxDaily": maxDaily()})
}

func (s *server) adsLaunch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 14<<20)
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		http.Error(w, "Upload not understood or the image is larger than 10 MB.", 400)
		return
	}
	f := func(k string) string { return strings.TrimSpace(r.FormValue(k)) }
	fail := func(m string) { http.Error(w, m, 400) }
	acct := strings.TrimPrefix(f("account"), "act_")
	if !reAct.MatchString(acct) {
		fail("Choose an ad account.")
		return
	}
	name, link := clip(f("name"), 80), f("link")
	if name == "" || !strings.HasPrefix(link, "https://") {
		fail("Enter a campaign name and a website address that starts with https://")
		return
	}
	budget, _ := strconv.ParseFloat(f("dailyBudget"), 64)
	if budget <= 0 || budget > maxDaily() {
		fail(fmt.Sprintf("Daily budget must be between 1 and %.0f. This safety cap can be raised with ADS_MAX_DAILY_BUDGET in the admin console.", maxDaily()))
		return
	}
	days, _ := strconv.Atoi(f("days"))
	if days < 1 || days > 60 {
		days = 7
	}
	obj, goal, dest := "OUTCOME_TRAFFIC", "LINK_CLICKS", "WEBSITE"
	if f("objective") == "awareness" {
		obj, goal, dest = "OUTCOME_AWARENESS", "REACH", ""
	}
	var countries []string
	for _, c := range strings.Split(f("countries"), ",") {
		if c = strings.ToUpper(strings.TrimSpace(c)); len(c) == 2 {
			countries = append(countries, c)
		}
	}
	if len(countries) == 0 {
		fail("Enter at least one 2-letter country code, for example ET or AE.")
		return
	}
	ageMin, _ := strconv.Atoi(f("ageMin"))
	ageMax, _ := strconv.Atoi(f("ageMax"))
	if ageMin < 18 || ageMin > 64 {
		ageMin = 18
	}
	if ageMax < ageMin || ageMax > 65 {
		ageMax = 65
	}
	file, hdr, err := r.FormFile("image")
	if err != nil {
		fail("Add an image for the ad (JPG or PNG).")
		return
	}
	defer file.Close()
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if !imageExt[ext] {
		fail("The ad image must be a JPG or PNG.")
		return
	}
	path := filepath.Join(mediaDir(), randHex(16)+ext)
	out, err := os.Create(path)
	if err != nil {
		fail("Server could not store the image.")
		return
	}
	_, cerr := io.Copy(out, file)
	out.Close()
	defer os.Remove(path)
	if cerr != nil {
		fail("The image upload failed.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	accts, tok, err := s.adsAccounts(ctx, uid)
	if err != nil {
		fail(err.Error())
		return
	}
	cur, found := "", false
	for _, a := range accts {
		if strings.TrimPrefix(a.ID, "act_") == acct {
			cur, found = a.Currency, true
		}
	}
	if !found {
		fail("That ad account is not available to your connection.")
		return
	}
	pid, _, _, err := s.fbPage(ctx, uid)
	if err != nil {
		fail("Connect your Facebook Page first (the ad is published from it).")
		return
	}
	igid := ""
	if t, e := s.validToken(ctx, uid, provOf("instagram")); e == nil {
		if id, e2 := igAccount(ctx, t.Access); e2 == nil {
			igid = id
		}
	}
	step := func(what string, err error) { fail("Meta refused the " + what + ": " + trunc(err.Error(), 320)) }
	base := gv + "act_" + acct + "/"

	var camp struct {
		ID string `json:"id"`
	}
	if err := postForm(ctx, base+"campaigns", url.Values{"access_token": {tok}, "name": {name}, "objective": {obj}, "status": {"PAUSED"}, "special_ad_categories": {"[]"}, "is_adset_budget_sharing_enabled": {"false"}}, &camp); err != nil {
		step("campaign", err)
		return
	}
	tgt := map[string]any{"geo_locations": map[string]any{"countries": countries}, "age_min": ageMin, "age_max": ageMax, "targeting_automation": map[string]any{"advantage_audience": 0}}
	switch f("gender") {
	case "male":
		tgt["genders"] = []int{1}
	case "female":
		tgt["genders"] = []int{2}
	}
	tb, _ := json.Marshal(tgt)
	start := time.Now().Add(20 * time.Minute)
	tf := "2006-01-02T15:04:05-0700"
	adsetForm := url.Values{"access_token": {tok}, "name": {name + " - ad set"}, "campaign_id": {camp.ID}, "daily_budget": {strconv.FormatInt(minorUnits(budget, cur), 10)},
		"billing_event": {"IMPRESSIONS"}, "optimization_goal": {goal}, "bid_strategy": {"LOWEST_COST_WITHOUT_CAP"}, "targeting": {string(tb)}, "status": {"PAUSED"},
		"start_time": {start.Format(tf)}, "end_time": {start.AddDate(0, 0, days).Format(tf)}}
	if dest != "" {
		adsetForm.Set("destination_type", dest)
	}
	var adset struct {
		ID string `json:"id"`
	}
	if err := postForm(ctx, base+"adsets", adsetForm, &adset); err != nil {
		step("ad set", err)
		return
	}
	var img struct {
		Images map[string]struct {
			Hash string `json:"hash"`
		} `json:"images"`
	}
	if err := postMultipart(ctx, base+"adimages", map[string]string{"access_token": tok}, "filename", path, &img); err != nil {
		step("image upload", err)
		return
	}
	hash := ""
	for _, v := range img.Images {
		hash = v.Hash
	}
	if hash == "" {
		fail("Meta did not return an image hash.")
		return
	}
	cta := f("cta")
	if cta == "" {
		cta = "LEARN_MORE"
	}
	spec := map[string]any{"page_id": pid, "link_data": map[string]any{"link": link, "message": cut(f("primary"), 125), "name": cut(f("headline"), 40), "description": cut(f("description"), 30), "image_hash": hash, "call_to_action": map[string]any{"type": cta, "value": map[string]any{"link": link}}}}
	sb, _ := json.Marshal(spec)
	crForm := url.Values{"access_token": {tok}, "name": {name + " - creative"}, "object_story_spec": {string(sb)}}
	if igid != "" {
		crForm.Set("instagram_user_id", igid)
	}
	var cr struct {
		ID string `json:"id"`
	}
	if err := postForm(ctx, base+"adcreatives", crForm, &cr); err != nil {
		step("ad creative", err)
		return
	}
	cb, _ := json.Marshal(map[string]string{"creative_id": cr.ID})
	var ad struct {
		ID string `json:"id"`
	}
	if err := postForm(ctx, base+"ads", url.Values{"access_token": {tok}, "name": {name + " - ad"}, "adset_id": {adset.ID}, "creative": {string(cb)}, "status": {"PAUSED"}}, &ad); err != nil {
		step("ad", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"campaignId": camp.ID, "adsetId": adset.ID, "adId": ad.ID, "account": acct, "currency": cur, "dailyBudget": budget, "status": "PAUSED",
		"summary": fmt.Sprintf("%s: %s per day for %d days, %s, ages %d-%d. Created PAUSED (nothing is spending yet).", name, strconv.FormatFloat(budget, 'f', 2, 64), days, strings.Join(countries, ", "), ageMin, ageMax)})
}

func (s *server) setStatus(ctx context.Context, tok, id, status string) error {
	var o map[string]any
	return postForm(ctx, gv+id, url.Values{"access_token": {tok}, "status": {status}}, &o)
}

func (s *server) adsAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in struct {
		Type    string  `json:"type"`
		ID      string  `json:"id"`
		Value   float64 `json:"value"`
		Account string  `json:"account"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in) != nil || !reAct.MatchString(in.ID) {
		http.Error(w, "bad request", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	accts, tok, err := s.adsAccounts(ctx, uid)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	switch in.Type {
	case "pause":
		err = s.setStatus(ctx, tok, in.ID, "PAUSED")
	case "activate":
		err = s.setStatus(ctx, tok, in.ID, "ACTIVE")
		var sets struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err == nil && getJSON(ctx, gv+in.ID+"/adsets?"+url.Values{"fields": {"id"}, "access_token": {tok}}.Encode(), &sets) == nil {
			for _, st := range sets.Data {
				if e := s.setStatus(ctx, tok, st.ID, "ACTIVE"); e != nil {
					err = e
					break
				}
				var ads struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
				}
				if getJSON(ctx, gv+st.ID+"/ads?"+url.Values{"fields": {"id"}, "access_token": {tok}}.Encode(), &ads) == nil {
					for _, a := range ads.Data {
						if e := s.setStatus(ctx, tok, a.ID, "ACTIVE"); e != nil {
							err = e
						}
					}
				}
			}
		}
	case "budget":
		cur := "USD"
		for _, a := range accts {
			if strings.TrimPrefix(a.ID, "act_") == strings.TrimPrefix(in.Account, "act_") {
				cur = a.Currency
			}
		}
		if in.Value <= 0 || in.Value > maxDaily() {
			http.Error(w, fmt.Sprintf("Budget must be between 1 and %.0f (safety cap).", maxDaily()), 400)
			return
		}
		var o map[string]any
		err = postForm(ctx, gv+in.ID, url.Values{"access_token": {tok}, "daily_budget": {strconv.FormatInt(minorUnits(in.Value, cur), 10)}}, &o)
	default:
		http.Error(w, "unknown action", 400)
		return
	}
	if err != nil {
		http.Error(w, "Meta refused the change: "+trunc(err.Error(), 300), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"ok":true}`))
}

type insRow struct {
	CampaignID   string `json:"campaign_id"`
	CampaignName string `json:"campaign_name"`
	AdID         string `json:"ad_id"`
	AdName       string `json:"ad_name"`
	AdsetID      string `json:"adset_id"`
	AdsetName    string `json:"adset_name"`
	Impr         string `json:"impressions"`
	Clicks       string `json:"clicks"`
	CTR          string `json:"ctr"`
	CPC          string `json:"cpc"`
	Spend        string `json:"spend"`
	Freq         string `json:"frequency"`
	Date         string `json:"date_start"`
	Actions      []struct {
		Type  string `json:"action_type"`
		Value string `json:"value"`
	} `json:"actions"`
}

func fetchIns(ctx context.Context, tok, acct, level, fields string, daily bool) []insRow {
	q := url.Values{"level": {level}, "date_preset": {"last_14d"}, "fields": {fields}, "limit": {"100"}, "access_token": {tok}}
	if daily {
		q.Set("time_increment", "1")
	}
	var d struct {
		Data []insRow `json:"data"`
	}
	if getJSON(ctx, gv+"act_"+acct+"/insights?"+q.Encode(), &d) != nil {
		return nil
	}
	return d.Data
}

func (s *server) evalAdsPerf(ctx context.Context, uid string, in evalIn) (*Report, error) {
	accts, tok, err := s.adsAccounts(ctx, uid)
	if err != nil {
		return nil, err
	}
	acct := strings.TrimPrefix(strings.TrimSpace(in.Site), "act_")
	cur := accts[0].Currency
	if !reAct.MatchString(acct) {
		acct = strings.TrimPrefix(accts[0].ID, "act_")
	}
	for _, a := range accts {
		if strings.TrimPrefix(a.ID, "act_") == acct {
			cur = a.Currency
		}
	}
	camps := fetchIns(ctx, tok, acct, "campaign", "campaign_id,campaign_name,impressions,clicks,ctr,cpc,spend,frequency,actions", false)
	ads := fetchIns(ctx, tok, acct, "ad", "ad_id,ad_name,adset_id,adset_name,campaign_name,impressions,clicks,ctr,cpc,spend,frequency,actions", false)
	days := fetchIns(ctx, tok, acct, "account", "impressions,clicks,spend", true)
	if len(camps) == 0 {
		return nil, userErr("No ad delivery in the last 14 days for this account yet. Activate a campaign, wait a few hours, then analyze again.")
	}
	r := &Report{Target: "Meta ads " + acct, Meta: map[string]string{"account": acct, "name": "Meta ads", "domain": acct}}
	var spend, impr, clicks, results float64
	for _, c := range camps {
		spend += atof(c.Spend)
		impr += atof(c.Impr)
		clicks += atof(c.Clicks)
		for _, a := range c.Actions {
			if a.Type == "link_click" {
				results += atof(a.Value)
			}
		}
	}
	ctr := 0.0
	cpc := 0.0
	if impr > 0 {
		ctr = clicks / impr * 100
	}
	if clicks > 0 {
		cpc = spend / clicks
	}
	r.KPIs = []KPI{{Label: "Spend (14 days)", Value: commas(spend) + " " + cur}, {Label: "Impressions", Value: commas(impr)}, {Label: "Clicks", Value: commas(clicks)},
		{Label: "CTR", Value: fmt.Sprintf("%.2f%%", ctr), Note: "Good \u2265 1%", Status: rate(ctr, 1, 0.5)}, {Label: "Cost per click", Value: fmt.Sprintf("%.2f %s", cpc, cur)}}
	if results > 0 {
		r.KPIs = append(r.KPIs, KPI{Label: "Link clicks (results)", Value: commas(results), Note: fmt.Sprintf("%.2f %s each", spend/results, cur)})
	}
	dc := Chart{Title: "Daily spend and clicks (14 days)", Type: "line", Datasets: []Dataset{{Name: "Spend"}, {Name: "Clicks"}}}
	for _, d := range days {
		dc.Labels = append(dc.Labels, d.Date)
		dc.Datasets[0].Data = append(dc.Datasets[0].Data, atof(d.Spend))
		dc.Datasets[1].Data = append(dc.Datasets[1].Data, atof(d.Clicks))
	}
	if len(dc.Labels) > 1 {
		r.Charts = append(r.Charts, dc)
	}
	sc := Chart{Title: "Spend by campaign", Type: "bar", Datasets: []Dataset{{Name: "Spend"}}}
	ct := Table{Title: "Campaigns (last 14 days)", Columns: []string{"Campaign", "Spend", "Impressions", "Clicks", "CTR", "CPC", "Frequency"}}
	for _, c := range camps {
		sc.Labels = append(sc.Labels, trunc(c.CampaignName, 22))
		sc.Datasets[0].Data = append(sc.Datasets[0].Data, atof(c.Spend))
		ct.Rows = append(ct.Rows, Row{Cells: []string{c.CampaignName, c.Spend, c.Impr, c.Clicks, c.CTR + "%", c.CPC, c.Freq}})
	}
	r.Charts = append(r.Charts, sc)
	at := Table{Title: "Ads (last 14 days)", Columns: []string{"Ad", "Campaign", "Spend", "Clicks", "CTR", "CPC", "Frequency"}}
	sort.Slice(ads, func(i, j int) bool { return atof(ads[i].Spend) > atof(ads[j].Spend) })
	cc := Chart{Title: "CTR by ad (%)", Type: "bar", Datasets: []Dataset{{Name: "CTR"}}}
	for _, a := range ads {
		at.Rows = append(at.Rows, Row{Cells: []string{a.AdName, a.CampaignName, a.Spend, a.Clicks, a.CTR + "%", a.CPC, a.Freq}})
		cc.Labels = append(cc.Labels, trunc(a.AdName, 20))
		cc.Datasets[0].Data = append(cc.Datasets[0].Data, math.Round(atof(a.CTR)*100)/100)
	}
	if len(ads) > 0 {
		r.Charts = append(r.Charts, cc)
	}
	r.Tables = []Table{ct, at}
	const C = "Ad performance"
	add := func(c Check) { r.Checks = append(r.Checks, c) }
	add(chk(C, "Click-through rate", rate(ctr, 1, 0.5), fmt.Sprintf("%.2f%%", ctr), "Test new images and headlines; narrow the audience; make the offer clearer.", 5))
	maxFreq := 0.0
	for _, a := range ads {
		maxFreq = math.Max(maxFreq, atof(a.Freq))
	}
	add(chk(C, "Ad frequency (max)", low(maxFreq, 3, 5), fmt.Sprintf("%.1f", maxFreq), "People see the same ad too often. Refresh the creative or widen the audience.", 4))
	add(chk(C, "Delivery", ifs(impr > 0, "pass", "fail"), commas(impr)+" impressions", "No delivery: check payment method, ad status and the ad review result in Ads Manager.", 5))
	r.finish()
	mc := maxDaily()
	if len(ads) > 0 {
		avgSpend := spend / float64(len(ads))
		for _, a := range ads {
			if atof(a.Spend) >= math.Max(avgSpend*0.5, 2) && atof(a.CTR) < 0.6 && a.AdID != "" {
				r.Plan = append(r.Plan, PlanItem{Title: fmt.Sprintf("Pause the weak ad \"%s\" (CTR %s%%)", trunc(a.AdName, 40), a.CTR), Why: "It has spent " + a.Spend + " " + cur + " with a CTR under 0.6%. Free that budget for better ads.", Impact: 4, Effort: 1, Auto: "ads:pause:" + a.AdID + ":" + acct})
			}
		}
		best := ads[0]
		for _, a := range ads {
			if atof(a.CTR) > atof(best.CTR) && atof(a.Spend) >= 1 {
				best = a
			}
		}
		if best.AdsetID != "" && atof(best.CTR) >= 1 {
			var as struct {
				Daily string `json:"daily_budget"`
			}
			if getJSON(ctx, gv+best.AdsetID+"?"+url.Values{"fields": {"daily_budget"}, "access_token": {tok}}.Encode(), &as) == nil {
				div := 100.0
				if zeroDecimal(cur) {
					div = 1
				}
				nb := math.Round(atof(as.Daily)/div*1.2*100) / 100
				if nb > 0 && nb <= mc {
					r.Plan = append(r.Plan, PlanItem{Title: fmt.Sprintf("Raise the budget of the best ad set (\"%s\", CTR %s%%) by 20%% to %.2f %s/day", trunc(best.AdsetName, 30), best.CTR, nb, cur), Why: "It is your highest-CTR ad set. Increase gradually and re-check in 3 days.", Impact: 4, Effort: 1, Auto: fmt.Sprintf("ads:budget:%s:%.2f:%s", best.AdsetID, nb, acct)})
				}
			}
		}
	}
	if maxFreq > 3 {
		r.Plan = append(r.Plan, PlanItem{Title: "Refresh the creative: frequency is above 3", Why: "People are seeing the same ad too often. Upload a new image and headline as a new ad.", Impact: 3, Effort: 2})
	}
	r.Plan = append(r.Plan, PlanItem{Title: "Re-run this analysis every 3 days and keep the report for the monthly review", Why: "Short feedback loops are how ad budgets improve.", Impact: 3, Effort: 1})
	if in.UseAI {
		r.AI = s.insights(ctx, r, "Meta ads performance for the last 14 days, currency "+cur+".")
	}
	r.Notes = append(r.Notes, "Data comes from the Meta Marketing API. Actions above change your live campaigns only when you press the button.")
	return r, nil
}
