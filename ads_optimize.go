package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Audience targeting and campaign optimisation actions for the user's own Meta ad account.
// Everything is previewed first (dry run), capped by ADS_MAX_DAILY_BUDGET, written to an audit log and returns
// the previous values so the screen can offer Undo.

func majorUnits(minor int64, cur string) float64 {
	if zeroDecimal(cur) {
		return float64(minor)
	}
	return float64(minor) / 100
}

type segment struct {
	Age     string  `json:"age"`
	Spend   float64 `json:"spend"`
	CTR     float64 `json:"ctr"`
	Results float64 `json:"results"`
	CPA     float64 `json:"cpa"`
}

type adsetView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Budget    float64   `json:"budget"`
	AgeMin    int       `json:"ageMin"`
	AgeMax    int       `json:"ageMax"`
	Countries []string  `json:"countries"`
	Advantage bool      `json:"advantage"`
	Spend     float64   `json:"spend"`
	Freq      float64   `json:"freq"`
	Segments  []segment `json:"segments"`
	SugMin    int       `json:"sugMin"`
	SugMax    int       `json:"sugMax"`
	Why       string    `json:"why"`
}

var ageBuckets = []struct {
	label    string
	min, max int
}{{"13-17", 13, 17}, {"18-24", 18, 24}, {"25-34", 25, 34}, {"35-44", 35, 44}, {"45-54", 45, 54}, {"55-64", 55, 64}, {"65+", 65, 65}}

func (s *server) adsAudit(uid, typ, id string, before, after any) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b1, _ := json.Marshal(before)
	b2, _ := json.Marshal(after)
	s.db.Collection("ads_actions").Doc(randHex(10)).Set(ctx, map[string]any{"uid": uid, "type": typ, "id": id, "before": string(b1), "after": string(b2), "at": time.Now().UTC()})
}

// suggest picks the age band that earns results most cheaply (or the best CTR when nothing converts).
func suggestBand(segs []segment, cur string) (int, int, string) {
	var total, spent float64
	for _, g := range segs {
		spent += g.Spend
		total += g.Results
	}
	if spent < 5 || len(segs) < 3 {
		return 18, 65, "There is not enough spend per age group yet to narrow the audience safely. Go broad (18-65) and let Meta find the buyers while you collect data."
	}
	score := func(g segment) float64 { // higher is better
		if total > 0 {
			if g.Results == 0 {
				return 0
			}
			return 1 / g.CPA
		}
		return g.CTR
	}
	best := -1
	for i, g := range segs {
		if g.Spend < spent*0.08 {
			continue
		}
		if best < 0 || score(g) > score(segs[best]) {
			best = i
		}
	}
	if best < 0 || score(segs[best]) == 0 {
		return 18, 65, "No age group stands out yet. Keep the audience broad and judge again after more spend."
	}
	lo, hi := best, best
	ok := func(g segment) bool { return g.Spend >= spent*0.05 && score(g) >= score(segs[best])*0.75 }
	for lo > 0 && ok(segs[lo-1]) {
		lo--
	}
	for hi < len(segs)-1 && ok(segs[hi+1]) {
		hi++
	}
	min, max := ageFrom(segs[lo].Age), ageTo(segs[hi].Age)
	if min < 18 {
		min = 18
	}
	var in, out segment
	for i, g := range segs {
		if i >= lo && i <= hi {
			in.Spend += g.Spend
			in.Results += g.Results
			in.CTR += g.CTR * g.Spend
		} else {
			out.Spend += g.Spend
			out.Results += g.Results
			out.CTR += g.CTR * g.Spend
		}
	}
	why := fmt.Sprintf("Ages %d-%d got %.0f%% of the spend", min, max, in.Spend/spent*100)
	if total > 0 && in.Results > 0 {
		why += fmt.Sprintf(" and delivered %.0f of %.0f results at %s %.2f each", in.Results, total, cur, in.Spend/in.Results)
		if out.Results > 0 {
			why += fmt.Sprintf(", against %s %.2f outside the band", cur, out.Spend/out.Results)
		}
	} else if in.Spend > 0 {
		why += fmt.Sprintf(" with an average CTR of %.2f%%", in.CTR/in.Spend)
	}
	return min, max, why + "."
}

func ageFrom(l string) int {
	for _, b := range ageBuckets {
		if b.label == l {
			return b.min
		}
	}
	return 18
}
func ageTo(l string) int {
	for _, b := range ageBuckets {
		if b.label == l {
			if b.max == 65 {
				return 65
			}
			return b.max
		}
	}
	return 65
}

func (s *server) loadAdsets(ctx context.Context, tok, parent, cur string) ([]adsetView, error) {
	var d struct {
		Data []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Status    string `json:"effective_status"`
			Budget    string `json:"daily_budget"`
			Targeting struct {
				AgeMin int `json:"age_min"`
				AgeMax int `json:"age_max"`
				Geo    struct {
					Countries []string `json:"countries"`
				} `json:"geo_locations"`
				Auto struct {
					Adv int `json:"advantage_audience"`
				} `json:"targeting_automation"`
			} `json:"targeting"`
		} `json:"data"`
	}
	q := url.Values{"fields": {"id,name,effective_status,daily_budget,targeting"}, "limit": {"25"}, "access_token": {tok}}
	if err := getJSON(ctx, gv+parent+"/adsets?"+q.Encode(), &d); err != nil {
		return nil, err
	}
	var out []adsetView
	for _, a := range d.Data {
		v := adsetView{ID: a.ID, Name: a.Name, Status: a.Status, AgeMin: a.Targeting.AgeMin, AgeMax: a.Targeting.AgeMax, Countries: a.Targeting.Geo.Countries, Advantage: a.Targeting.Auto.Adv == 1}
		if b, err := strconv.ParseInt(a.Budget, 10, 64); err == nil {
			v.Budget = majorUnits(b, cur)
		}
		out = append(out, v)
	}
	// age breakdown for the six biggest spenders
	for i := range out {
		if i >= 6 {
			break
		}
		var ins struct {
			Data []struct {
				Age     string   `json:"age"`
				Spend   string   `json:"spend"`
				CTR     string   `json:"ctr"`
				Freq    string   `json:"frequency"`
				Actions []actVal `json:"actions"`
			} `json:"data"`
		}
		iq := url.Values{"fields": {"spend,ctr,frequency,actions"}, "breakdowns": {"age"}, "date_preset": {"last_30d"}, "limit": {"20"}, "access_token": {tok}}
		if getJSON(ctx, gv+out[i].ID+"/insights?"+iq.Encode(), &ins) != nil {
			continue
		}
		agg := map[string]*segment{}
		for _, r := range ins.Data {
			g := agg[r.Age]
			if g == nil {
				g = &segment{Age: r.Age}
				agg[r.Age] = g
			}
			sp := atof(r.Spend)
			g.Spend += sp
			g.CTR = atof(r.CTR)
			res := pickAct(r.Actions, purchaseTypes)
			if res == 0 {
				res = pickAct(r.Actions, leadTypes)
			}
			g.Results += res
			out[i].Spend += sp
			if f := atof(r.Freq); f > out[i].Freq {
				out[i].Freq = f
			}
		}
		for _, b := range ageBuckets {
			if g := agg[b.label]; g != nil {
				if g.Results > 0 {
					g.CPA = g.Spend / g.Results
				}
				out[i].Segments = append(out[i].Segments, *g)
			}
		}
		out[i].SugMin, out[i].SugMax, out[i].Why = suggestBand(out[i].Segments, cur)
		if out[i].Freq > 3 {
			out[i].SugMin, out[i].SugMax = 18, 65
			out[i].Why = fmt.Sprintf("People see this ad %.1f times on average. Widen the audience (ages 18-65 with Advantage+ audience) so new people are reached before you add more budget.", out[i].Freq)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Spend > out[j].Spend })
	return out, nil
}

// GET /api/ads/targeting?account=123&id=<campaign id, optional>
func (s *server) adsTargeting(w http.ResponseWriter, r *http.Request) {
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	accts, tok, err := s.adsAccounts(ctx, uid)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	acct := strings.TrimPrefix(r.URL.Query().Get("account"), "act_")
	if !reAct.MatchString(acct) {
		acct = strings.TrimPrefix(accts[0].ID, "act_")
	}
	cur := accts[0].Currency
	for _, a := range accts {
		if strings.TrimPrefix(a.ID, "act_") == acct {
			cur = a.Currency
		}
	}
	parent := "act_" + acct
	if id := r.URL.Query().Get("id"); reAct.MatchString(id) {
		parent = id
	}
	sets, err := s.loadAdsets(ctx, tok, parent, cur)
	if err != nil {
		http.Error(w, "Meta did not return the ad sets: "+trunc(err.Error(), 240), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"currency": cur, "cap": maxDaily(), "adsets": sets})
}

// POST /api/ads/optimize  {type: scale|audience|test|start, id, pct, ageMin, ageMax, advantage, account, dry}
func (s *server) adsOptimize(w http.ResponseWriter, r *http.Request) {
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
		Type      string  `json:"type"`
		ID        string  `json:"id"`
		Pct       float64 `json:"pct"`
		AgeMin    int     `json:"ageMin"`
		AgeMax    int     `json:"ageMax"`
		Advantage bool    `json:"advantage"`
		Account   string  `json:"account"`
		Dry       bool    `json:"dry"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in) != nil || !reAct.MatchString(in.ID) {
		http.Error(w, "bad request", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	accts, tok, err := s.adsAccounts(ctx, uid)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	cur := accts[0].Currency
	for _, a := range accts {
		if strings.TrimPrefix(a.ID, "act_") == strings.TrimPrefix(in.Account, "act_") {
			cur = a.Currency
		}
	}
	fail := func(m string) { http.Error(w, m, 400) }
	reply := func(v map[string]any) {
		v["ok"] = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
	switch in.Type {
	case "scale":
		pct := math.Min(math.Max(in.Pct, 1), 30)
		// the object itself (CBO campaign or ad set), else every ad set below it
		type tgt struct {
			id   string
			name string
			old  int64
		}
		var list []tgt
		var o struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Budget string `json:"daily_budget"`
		}
		q := url.Values{"fields": {"id,name,daily_budget"}, "access_token": {tok}}
		if err := getJSON(ctx, gv+in.ID+"?"+q.Encode(), &o); err != nil {
			fail("Meta did not return this campaign: " + trunc(err.Error(), 240))
			return
		}
		if b, _ := strconv.ParseInt(o.Budget, 10, 64); b > 0 {
			list = append(list, tgt{o.ID, o.Name, b})
		} else {
			var d struct {
				Data []struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Budget string `json:"daily_budget"`
					Status string `json:"effective_status"`
				} `json:"data"`
			}
			if getJSON(ctx, gv+in.ID+"/adsets?"+url.Values{"fields": {"id,name,daily_budget,effective_status"}, "limit": {"25"}, "access_token": {tok}}.Encode(), &d) == nil {
				for _, a := range d.Data {
					if b, _ := strconv.ParseInt(a.Budget, 10, 64); b > 0 && a.Status == "ACTIVE" {
						list = append(list, tgt{a.ID, a.Name, b})
					}
				}
			}
		}
		if len(list) == 0 {
			fail("This campaign has no daily budget to raise (it may use a lifetime budget). Change it in Ads Manager.")
			return
		}
		var rows []map[string]any
		var undo []map[string]any
		for _, t := range list {
			from := majorUnits(t.old, cur)
			to := math.Round(from*(1+pct/100)*100) / 100
			if to > maxDaily() {
				fail(fmt.Sprintf("%s would go to %.2f %s a day, above the safety cap of %.0f. Raise ADS_MAX_DAILY_BUDGET in Admin if you really want this.", t.name, to, cur, maxDaily()))
				return
			}
			rows = append(rows, map[string]any{"id": t.id, "name": t.name, "from": from, "to": to})
			undo = append(undo, map[string]any{"id": t.id, "value": from})
		}
		if !in.Dry {
			for i, t := range list {
				var res map[string]any
				if err := postForm(ctx, gv+t.id, url.Values{"access_token": {tok}, "daily_budget": {strconv.FormatInt(minorUnits(rows[i]["to"].(float64), cur), 10)}}, &res); err != nil {
					fail("Meta refused the change: " + trunc(err.Error(), 300))
					return
				}
			}
			s.adsAudit(uid, "scale", in.ID, undo, rows)
		}
		reply(map[string]any{"rows": rows, "undo": undo, "currency": cur, "dry": in.Dry})
	case "audience", "test":
		if in.AgeMin < 18 || in.AgeMax > 65 || in.AgeMin >= in.AgeMax {
			fail("Choose an age range between 18 and 65.")
			return
		}
		if in.Advantage {
			if in.AgeMin > 25 {
				in.AgeMin = 25
			}
			in.AgeMax = 65
		}
		target := in.ID
		var raw map[string]any
		if err := getJSON(ctx, gv+in.ID+"?"+url.Values{"fields": {"name,targeting"}, "access_token": {tok}}.Encode(), &raw); err != nil {
			fail("Meta did not return this ad set: " + trunc(err.Error(), 240))
			return
		}
		tg, _ := raw["targeting"].(map[string]any)
		if tg == nil {
			fail("This ad set has no readable targeting.")
			return
		}
		before := map[string]any{"ageMin": tg["age_min"], "ageMax": tg["age_max"], "advantage": false}
		if au, ok := tg["targeting_automation"].(map[string]any); ok {
			before["advantage"] = au["advantage_audience"] == float64(1)
		}
		after := map[string]any{"ageMin": in.AgeMin, "ageMax": in.AgeMax, "advantage": in.Advantage}
		if in.Dry {
			reply(map[string]any{"before": before, "after": after, "dry": true})
			return
		}
		if in.Type == "test" {
			var cp struct {
				Copied string `json:"copied_adset_id"`
				Ids    []struct {
					Old string `json:"source_adset_id"`
					New string `json:"copied_adset_id"`
				} `json:"ad_object_ids"`
			}
			if err := postForm(ctx, gv+in.ID+"/copies", url.Values{"access_token": {tok}, "deep_copy": {"true"}, "status_option": {"PAUSED"}, "rename_options": {`{"rename_suffix":" - audience test"}`}}, &cp); err != nil {
				fail("Meta refused to copy the ad set: " + trunc(err.Error(), 300))
				return
			}
			target = cp.Copied
			if target == "" && len(cp.Ids) > 0 {
				target = cp.Ids[0].New
			}
			if !reAct.MatchString(target) {
				fail("Meta did not return the copied ad set. Check Ads Manager before trying again.")
				return
			}
		}
		tg["age_min"], tg["age_max"] = in.AgeMin, in.AgeMax
		delete(tg, "age_range")
		auto, _ := tg["targeting_automation"].(map[string]any)
		if auto == nil {
			auto = map[string]any{}
		}
		if in.Advantage {
			auto["advantage_audience"] = 1
		} else {
			auto["advantage_audience"] = 0
		}
		tg["targeting_automation"] = auto
		tb, _ := json.Marshal(tg)
		var res map[string]any
		if err := postForm(ctx, gv+target, url.Values{"access_token": {tok}, "targeting": {string(tb)}}, &res); err != nil {
			fail("Meta refused the new audience: " + trunc(err.Error(), 300))
			return
		}
		s.adsAudit(uid, in.Type, target, before, after)
		reply(map[string]any{"before": before, "after": after, "id": target, "copy": in.Type == "test"})
	case "start":
		if err := s.setStatus(ctx, tok, in.ID, "ACTIVE"); err != nil {
			fail("Meta refused: " + trunc(err.Error(), 300))
			return
		}
		var ads struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if getJSON(ctx, gv+in.ID+"/ads?"+url.Values{"fields": {"id"}, "access_token": {tok}}.Encode(), &ads) == nil {
			for _, a := range ads.Data {
				s.setStatus(ctx, tok, a.ID, "ACTIVE")
			}
		}
		s.adsAudit(uid, "start", in.ID, nil, "ACTIVE")
		reply(map[string]any{})
	default:
		fail("unknown action")
	}
}
