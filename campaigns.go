package main

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// "Campaigns managed" report: every Meta campaign in the ad account with spend, results, CPA, conversion rate,
// ROAS and the winning ads, written up so it can be copied straight into a portfolio or a client report.

type actVal struct {
	Type  string `json:"action_type"`
	Value string `json:"value"`
}

type perfRow struct {
	CampaignID   string   `json:"campaign_id"`
	CampaignName string   `json:"campaign_name"`
	AdID         string   `json:"ad_id"`
	AdName       string   `json:"ad_name"`
	Impr         string   `json:"impressions"`
	Clicks       string   `json:"clicks"`
	LinkClicks   string   `json:"inline_link_clicks"`
	Spend        string   `json:"spend"`
	Freq         string   `json:"frequency"`
	Actions      []actVal `json:"actions"`
	Values       []actVal `json:"action_values"`
}

// Meta reports the same purchase under several action types; the first type found is used so nothing is counted twice.
var (
	purchaseTypes = []string{"omni_purchase", "purchase", "offsite_conversion.fb_pixel_purchase"}
	leadTypes     = []string{"lead", "onsite_conversion.lead_grouped", "offsite_conversion.fb_pixel_lead", "omni_complete_registration", "complete_registration"}
	periodOK      = map[string]string{"last_30d": "last 30 days", "last_90d": "last 90 days", "this_year": "this year", "last_year": "last year", "maximum": "all time"}
)

func pickAct(list []actVal, types []string) float64 {
	for _, t := range types {
		for _, a := range list {
			if a.Type == t {
				return atof(a.Value)
			}
		}
	}
	return 0
}

type perf struct {
	ID, Name, Camp, Obj, Status, Label       string
	Spend, Impr, Clicks, Link, Results, Value float64
	Freq                                      float64
}

func (p perf) base() float64 {
	if p.Link > 0 {
		return p.Link
	}
	return p.Clicks
}
func (p perf) cpa() float64 {
	if p.Results > 0 {
		return p.Spend / p.Results
	}
	return 0
}
func (p perf) roas() float64 {
	if p.Spend > 0 && p.Value > 0 {
		return p.Value / p.Spend
	}
	return 0
}
func (p perf) cvr() float64 {
	if p.base() > 0 {
		return p.Results / p.base() * 100
	}
	return 0
}
func (p perf) ctr() float64 {
	if p.Impr > 0 {
		return p.Clicks / p.Impr * 100
	}
	return 0
}

func toPerf(r perfRow) perf {
	p := perf{ID: r.AdID, Name: r.AdName, Camp: r.CampaignName, Spend: atof(r.Spend), Impr: atof(r.Impr), Clicks: atof(r.Clicks), Link: atof(r.LinkClicks), Freq: atof(r.Freq)}
	if v := pickAct(r.Actions, purchaseTypes); v > 0 {
		p.Results, p.Value, p.Label = v, pickAct(r.Values, purchaseTypes), "purchases"
	} else if v := pickAct(r.Actions, leadTypes); v > 0 {
		p.Results, p.Label = v, "leads"
	}
	return p
}

func fetchPerf(ctx context.Context, tok, acct, level, preset string) ([]perfRow, error) {
	fields := "campaign_id,campaign_name,impressions,clicks,inline_link_clicks,spend,frequency,actions,action_values"
	if level == "ad" {
		fields = "ad_id,ad_name," + fields
	}
	var d struct {
		Data []perfRow `json:"data"`
	}
	q := url.Values{"level": {level}, "date_preset": {preset}, "fields": {fields}, "limit": {"200"}, "access_token": {tok}}
	if err := getJSON(ctx, gv+"act_"+acct+"/insights?"+q.Encode(), &d); err != nil {
		return nil, err
	}
	return d.Data, nil
}

func niceObjective(o string) string {
	o = strings.TrimPrefix(strings.ToUpper(o), "OUTCOME_")
	o = strings.ToLower(strings.ReplaceAll(o, "_", " "))
	if o == "" {
		return "-"
	}
	return strings.ToUpper(o[:1]) + o[1:]
}

func money(v float64, cur string) string { return fmt.Sprintf("%s %s", commas2(v), cur) }

func commas2(v float64) string {
	if v >= 100 {
		return commas(v)
	}
	return fmt.Sprintf("%.2f", v)
}

func dash(v float64, f string) string {
	if v <= 0 {
		return "–"
	}
	return fmt.Sprintf(f, v)
}

func (s *server) evalCampaigns(ctx context.Context, uid string, in evalIn) (*Report, error) {
	accts, tok, err := s.adsAccounts(ctx, uid)
	if err != nil {
		return nil, err
	}
	acct := strings.TrimPrefix(strings.TrimSpace(in.Site), "act_")
	if !reAct.MatchString(acct) {
		acct = strings.TrimPrefix(accts[0].ID, "act_")
	}
	cur := accts[0].Currency
	for _, a := range accts {
		if strings.TrimPrefix(a.ID, "act_") == acct {
			cur = a.Currency
		}
	}
	preset := strings.TrimSpace(in.Query)
	if _, ok := periodOK[preset]; !ok {
		preset = "last_30d"
	}
	period := periodOK[preset]

	crows, err := fetchPerf(ctx, tok, acct, "campaign", preset)
	if err != nil {
		return nil, userErr("Could not read your Meta campaigns (" + trunc(err.Error(), 160) + "). Reconnect Instagram or Facebook and approve ads_read.")
	}
	if len(crows) == 0 {
		return nil, userErr("No ad delivery in the " + period + " for this account. Pick a longer period, or run a campaign first.")
	}
	arows, _ := fetchPerf(ctx, tok, acct, "ad", preset)

	// objective and status come from the campaigns list
	type cm struct{ Obj, Status string }
	meta := map[string]cm{}
	var cl struct {
		Data []struct {
			ID        string `json:"id"`
			Objective string `json:"objective"`
			Status    string `json:"effective_status"`
		} `json:"data"`
	}
	if getJSON(ctx, gv+"act_"+acct+"/campaigns?"+url.Values{"fields": {"id,objective,effective_status"}, "limit": {"200"}, "access_token": {tok}}.Encode(), &cl) == nil {
		for _, c := range cl.Data {
			meta[c.ID] = cm{niceObjective(c.Objective), strings.ToLower(strings.ReplaceAll(c.Status, "_", " "))}
		}
	}

	var camps, ads []perf
	var tot perf
	for _, r := range crows {
		p := toPerf(r)
		p.ID, p.Name, p.Camp = r.CampaignID, r.CampaignName, r.CampaignName
		p.Obj, p.Status = meta[r.CampaignID].Obj, meta[r.CampaignID].Status
		if p.Obj == "" {
			p.Obj = "-"
		}
		if p.Status == "" {
			p.Status = "-"
		}
		camps = append(camps, p)
		tot.Spend += p.Spend
		tot.Impr += p.Impr
		tot.Clicks += p.Clicks
		tot.Link += p.Link
		tot.Results += p.Results
		tot.Value += p.Value
		if tot.Label == "" {
			tot.Label = p.Label
		} else if p.Label != "" && p.Label != tot.Label {
			tot.Label = "results"
		}
	}
	if tot.Label == "" {
		tot.Label = "results"
	}
	for _, r := range arows {
		ads = append(ads, toPerf(r))
	}
	sort.Slice(camps, func(i, j int) bool { return camps[i].Spend > camps[j].Spend })
	sort.Slice(ads, func(i, j int) bool { return ads[i].Spend > ads[j].Spend })

	hasValue := tot.Value > 0
	r := &Report{Target: "Meta campaigns " + acct, Meta: map[string]string{"account": acct, "name": "Meta campaigns", "domain": acct, "period": period}}

	// KPIs
	r.KPIs = []KPI{{Label: "Campaigns managed", Value: fmt.Sprintf("%d", len(camps)), Note: period}, {Label: "Total spend", Value: money(tot.Spend, cur)}}
	if hasValue {
		r.KPIs = append(r.KPIs, KPI{Label: "Revenue tracked", Value: money(tot.Value, cur)}, KPI{Label: "ROAS", Value: fmt.Sprintf("%.2fx", tot.roas()), Note: "revenue per 1 spent", Status: rate(tot.roas(), 3, 1.5)})
	}
	if tot.Results > 0 {
		r.KPIs = append(r.KPIs, KPI{Label: strings.Title(tot.Label), Value: commas(tot.Results)}, KPI{Label: "CPA", Value: money(tot.cpa(), cur), Note: "cost per " + strings.TrimSuffix(tot.Label, "s")},
			KPI{Label: "Conversion rate", Value: fmt.Sprintf("%.2f%%", tot.cvr()), Note: "results per link click"})
	}
	r.KPIs = append(r.KPIs, KPI{Label: "CTR", Value: fmt.Sprintf("%.2f%%", tot.ctr()), Note: "Good ≥ 1%", Status: rate(tot.ctr(), 1, 0.5)})

	// Campaign table, charts and written documentation
	ct := Table{Title: "Campaigns managed", Columns: []string{"Campaign", "Objective", "Status", "Spend", "Results", "CPA", "Conv. rate", "ROAS", "CTR"}}
	sp := Chart{Title: "Spend by campaign", Type: "bar", Datasets: []Dataset{{Name: "Spend"}}}
	rc := Chart{Title: "ROAS by campaign", Type: "bar", Datasets: []Dataset{{Name: "ROAS"}}}
	cc := Chart{Title: "CPA by campaign", Type: "bar", Datasets: []Dataset{{Name: "CPA"}}}
	for _, c := range camps {
		res := "–"
		if c.Results > 0 {
			res = fmt.Sprintf("%s %s", commas(c.Results), c.Label)
		}
		ct.Rows = append(ct.Rows, Row{Cells: []string{c.Name, c.Obj, c.Status, money(c.Spend, cur), res, dash(c.cpa(), "%.2f"), dash(c.cvr(), "%.2f%%"), dash(c.roas(), "%.2fx"), fmt.Sprintf("%.2f%%", c.ctr())}})
		sp.Labels = append(sp.Labels, trunc(c.Name, 22))
		sp.Datasets[0].Data = append(sp.Datasets[0].Data, c.Spend)
		if c.roas() > 0 {
			rc.Labels = append(rc.Labels, trunc(c.Name, 22))
			rc.Datasets[0].Data = append(rc.Datasets[0].Data, float64(int(c.roas()*100+0.5))/100)
		}
		if c.cpa() > 0 {
			cc.Labels = append(cc.Labels, trunc(c.Name, 22))
			cc.Datasets[0].Data = append(cc.Datasets[0].Data, float64(int(c.cpa()*100+0.5))/100)
		}
	}
	r.Charts = append(r.Charts, sp)
	if len(rc.Labels) > 0 {
		r.Charts = append(r.Charts, rc)
	}
	if len(cc.Labels) > 0 {
		r.Charts = append(r.Charts, cc)
	}

	// Winning ads: best ROAS when revenue is tracked, otherwise lowest CPA, otherwise best CTR.
	avgSpend := 0.0
	if len(ads) > 0 {
		avgSpend = tot.Spend / float64(len(ads))
	}
	var cand []perf
	for _, a := range ads {
		if a.Spend >= avgSpend*0.3 && a.Spend > 0 {
			cand = append(cand, a)
		}
	}
	basis := "CTR"
	switch {
	case hasValue:
		basis = "ROAS"
		sort.SliceStable(cand, func(i, j int) bool { return cand[i].roas() > cand[j].roas() })
	case tot.Results > 0:
		basis = "CPA"
		sort.SliceStable(cand, func(i, j int) bool {
			a, b := cand[i], cand[j]
			if (a.Results > 0) != (b.Results > 0) {
				return a.Results > 0
			}
			if a.Results == 0 {
				return a.ctr() > b.ctr()
			}
			return a.cpa() < b.cpa()
		})
	default:
		sort.SliceStable(cand, func(i, j int) bool { return cand[i].ctr() > cand[j].ctr() })
	}
	wt := Table{Title: "Winning ads (ranked by " + basis + ")", Columns: []string{"Ad", "Campaign", "Spend", "Results", "CPA", "Conv. rate", "ROAS", "CTR"}}
	var winners []perf
	for i, a := range cand {
		if i >= 8 {
			break
		}
		res := "–"
		if a.Results > 0 {
			res = commas(a.Results)
		}
		wt.Rows = append(wt.Rows, Row{Cells: []string{a.Name, a.Camp, money(a.Spend, cur), res, dash(a.cpa(), "%.2f"), dash(a.cvr(), "%.2f%%"), dash(a.roas(), "%.2fx"), fmt.Sprintf("%.2f%%", a.ctr())}})
		winners = append(winners, a)
	}
	// best ad per campaign for the documentation
	bestOf := map[string]perf{}
	for _, a := range cand {
		if b, ok := bestOf[a.Camp]; !ok {
			bestOf[a.Camp] = a
		} else if (hasValue && a.roas() > b.roas()) || (!hasValue && a.Results > 0 && (b.Results == 0 || a.cpa() < b.cpa())) {
			bestOf[a.Camp] = a
		}
	}
	dt := Table{Title: "Campaign documentation (copy into your portfolio or client report)", Columns: []string{"Campaign", "What ran", "Result", "Winning ad", "Next test"}}
	for _, c := range camps {
		nads := 0
		for _, a := range ads {
			if a.Camp == c.Name {
				nads++
			}
		}
		ran := fmt.Sprintf("%s campaign, %s, %d ad%s, %s", c.Obj, c.Status, nads, ifs(nads == 1, "", "s"), period)
		res := fmt.Sprintf("Spent %s", money(c.Spend, cur))
		if c.Results > 0 {
			res += fmt.Sprintf(", %s %s at %s CPA, %.2f%% conversion rate", commas(c.Results), c.Label, money(c.cpa(), cur), c.cvr())
		}
		if c.roas() > 0 {
			res += fmt.Sprintf(", ROAS %.2fx", c.roas())
		}
		if c.Results == 0 {
			res += fmt.Sprintf(", CTR %.2f%% (no purchase or lead events tracked)", c.ctr())
		}
		win := "–"
		if b, ok := bestOf[c.Name]; ok {
			win = b.Name
			if b.roas() > 0 {
				win += fmt.Sprintf(" (ROAS %.2fx, CTR %.2f%%)", b.roas(), b.ctr())
			} else if b.Results > 0 {
				win += fmt.Sprintf(" (CPA %s, CTR %.2f%%)", money(b.cpa(), cur), b.ctr())
			} else {
				win += fmt.Sprintf(" (CTR %.2f%%)", b.ctr())
			}
		}
		next := "Test a new hook on the winning angle against the current best ad."
		switch {
		case c.Freq > 3:
			next = "Frequency is above 3: launch fresh creative to the same audience."
		case hasValue && c.roas() >= 3:
			next = "Strong ROAS: raise the budget about 20% every 3 days while ROAS holds."
		case hasValue && c.roas() > 0 && c.roas() < 1.5:
			next = "ROAS is low: pause the weakest ads and test a stronger offer or angle."
		case c.Results == 0 && c.Spend > 0:
			next = "Fix conversion tracking (Pixel / Conversions API), then judge it on CPA and ROAS."
		}
		dt.Rows = append(dt.Rows, Row{Cells: []string{c.Name, ran, res, win, next}})
	}
	r.Tables = []Table{ct, wt, dt}

	// checks
	const C = "Ad performance"
	add := func(c Check) { r.Checks = append(r.Checks, c) }
	add(chk(C, "Conversion tracking", ifs(tot.Results > 0, "pass", "fail"), ifs(tot.Results > 0, commas(tot.Results)+" "+tot.Label+" tracked", "No purchase or lead events"),
		"Install the Meta Pixel and Conversions API so purchases and leads are counted. Without them ROAS and CPA cannot be measured.", 5))
	if hasValue {
		add(chk(C, "ROAS", rate(tot.roas(), 3, 1.5), fmt.Sprintf("%.2fx", tot.roas()), "Cut or rework the lowest-ROAS ads, then move budget to the winning ads and offers.", 5))
	}
	if tot.Results > 0 {
		add(chk(C, "Conversion rate", rate(tot.cvr(), 2, 0.8), fmt.Sprintf("%.2f%%", tot.cvr()), "Improve the landing page speed and offer match, and send traffic only to the page the ad promises.", 4))
	}
	add(chk(C, "Click-through rate", rate(tot.ctr(), 1, 0.5), fmt.Sprintf("%.2f%%", tot.ctr()), "Test new hooks in the first 3 seconds and a clearer offer in the headline.", 4))
	maxFreq := 0.0
	for _, c := range camps {
		if c.Freq > maxFreq {
			maxFreq = c.Freq
		}
	}
	add(chk(C, "Ad frequency (max)", low(maxFreq, 3, 5), fmt.Sprintf("%.1f", maxFreq), "People see the same ad too often. Refresh the creative or widen the audience.", 3))
	r.finish()

	// plan
	for i, w := range winners {
		if i >= 3 {
			break
		}
		why := "Spent " + money(w.Spend, cur) + ", CTR " + fmt.Sprintf("%.2f%%", w.ctr())
		if w.roas() > 0 {
			why += fmt.Sprintf(", ROAS %.2fx", w.roas())
		}
		if w.Results > 0 {
			why += ", CPA " + money(w.cpa(), cur)
		}
		why += ". Keep its angle and hook; change the opening and the visual."
		r.Plan = append(r.Plan, PlanItem{Title: "Brief and script 3 new ads from the winner \"" + trunc(w.Name, 40) + "\"", Why: why, Impact: 5, Effort: 2, Auto: "adbrief:" + trunc(w.Name, 60) + " | " + trunc(w.Camp, 40) + " | " + why})
	}
	avgCPA := tot.cpa()
	for _, a := range ads {
		if a.ID == "" || a.Spend < avgSpend*0.5 || a.Spend < 2 {
			continue
		}
		weak := (tot.Results > 0 && a.Results == 0) || (avgCPA > 0 && a.cpa() > avgCPA*1.5) || (tot.Results == 0 && a.ctr() < 0.6)
		if weak {
			r.Plan = append(r.Plan, PlanItem{Title: "Pause the weak ad \"" + trunc(a.Name, 40) + "\"", Why: "Spent " + money(a.Spend, cur) + " with " + ifs(a.Results > 0, "a CPA of "+money(a.cpa(), cur)+" (account average "+money(avgCPA, cur)+")", "no tracked results") + ". Move that budget to the winning ads.", Impact: 4, Effort: 1, Auto: "ads:pause:" + a.ID + ":" + acct})
			if len(r.Plan) >= 8 {
				break
			}
		}
	}
	r.Plan = append(r.Plan, PlanItem{Title: "Document each campaign and re-run this report monthly", Why: "Keep the Campaign documentation table as your running record of what ran, what it returned and what you test next.", Impact: 3, Effort: 1})
	if in.UseAI {
		r.AI = s.insights(ctx, r, "Meta campaigns managed in the "+period+", currency "+cur+". Judge them by ROAS, CPA and conversion rate and name the winning ads.")
	}
	r.Notes = append(r.Notes, "Data comes from the Meta Marketing API for the "+period+". ROAS = tracked purchase revenue / spend; CPA = spend / purchases (or leads); conversion rate = results / link clicks. Meta reports up to 200 campaigns and ads per period.")
	return r, nil
}
