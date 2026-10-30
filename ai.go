package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func cleanJSON(s string) string {
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j < i {
		return s
	}
	return s[i : j+1]
}

func (s *server) gemini(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"contents":         []any{map[string]any{"parts": []any{map[string]string{"text": prompt}}}},
		"generationConfig": map[string]any{"responseMimeType": "application/json"},
	})
	url := "https://generativelanguage.googleapis.com/v1beta/models/" + geminiModel() + ":generateContent"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", env("GEMINI_API_KEY"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", errRate
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini status %d", resp.StatusCode)
	}
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("empty gemini reply")
	}
	return out.Candidates[0].Content.Parts[0].Text, nil
}

type visResult struct {
	Mentions, Total int
	Category        string
	Rivals          []Point
}

func brandToken(domain string) string {
	p := strings.Split(domain, ".")
	return strings.ReplaceAll(p[0], "-", "")
}

// aiVisibility measures how often the brand shows up when an AI answers buyer questions
// (two calls: generate questions without the brand, then answer them without the brand).
func (s *server) aiVisibility(ctx context.Context, domain, title, desc string) (*visResult, error) {
	p1 := fmt.Sprintf("Website: %s\nTitle: %s\nDescription: %s\n\nReturn JSON only: {\"category\":\"short market category\",\"questions\":[five different questions a potential customer would type into an AI assistant when looking for this kind of product or service; never include the brand or domain name]}", clip(domain, 80), clip(title, 150), clip(desc, 250))
	t1, err := s.gemini(ctx, p1)
	if err != nil {
		return nil, err
	}
	var q struct {
		Category  string   `json:"category"`
		Questions []string `json:"questions"`
	}
	if json.Unmarshal([]byte(cleanJSON(t1)), &q) != nil || len(q.Questions) == 0 {
		return nil, errors.New("bad questions")
	}
	if len(q.Questions) > 5 {
		q.Questions = q.Questions[:5]
	}
	var sb strings.Builder
	for i, x := range q.Questions {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, clip(x, 200))
	}
	p2 := "You are a helpful AI assistant. For each numbered question, list up to 5 specific brands, products or websites you would recommend, best first. Return JSON only: {\"answers\":[[\"name\"],[\"name\"]]} with one inner list per question, in order.\n\n" + sb.String()
	t2, err := s.gemini(ctx, p2)
	if err != nil {
		return nil, err
	}
	var a struct {
		Answers [][]string `json:"answers"`
	}
	if json.Unmarshal([]byte(cleanJSON(t2)), &a) != nil || len(a.Answers) == 0 {
		return nil, errors.New("bad answers")
	}
	brand := brandToken(domain)
	res := &visResult{Total: len(q.Questions), Category: q.Category}
	count := map[string]int{}
	for _, list := range a.Answers {
		hit := false
		for _, it := range list {
			n := strings.ToLower(strings.TrimSpace(it))
			if n == "" {
				continue
			}
			flat := strings.ReplaceAll(strings.ReplaceAll(n, " ", ""), "-", "")
			match := strings.Contains(n, domain) || flat == brand
			if len(brand) >= 4 && strings.Contains(flat, brand) {
				match = true
			}
			if match {
				hit = true
				continue
			}
			count[n]++
		}
		if hit {
			res.Mentions++
		}
	}
	type kv struct {
		k string
		v int
	}
	var kvs []kv
	for k, v := range count {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].v != kvs[j].v {
			return kvs[i].v > kvs[j].v
		}
		return kvs[i].k < kvs[j].k
	})
	for i := 0; i < len(kvs) && i < 6; i++ {
		res.Rivals = append(res.Rivals, Point{Label: kvs[i].k, Value: float64(kvs[i].v)})
	}
	return res, nil
}

// insights turns the measured data into a written strategy (one Gemini call).
func (s *server) insights(ctx context.Context, r *Report, extra string) *AIInsights {
	var b strings.Builder
	fmt.Fprintf(&b, "Type: %s\nTarget: %s\nOverall score: %d/100\n", r.Kind, clip(r.Target, 80), r.Score)
	for _, k := range r.KPIs {
		fmt.Fprintf(&b, "KPI %s = %s (%s)\n", k.Label, k.Value, k.Status)
	}
	n := 0
	for _, c := range r.Checks {
		if c.Status != "pass" && n < 10 {
			fmt.Fprintf(&b, "Issue [%s] %s: %s\n", c.Status, c.Name, c.Value)
			n++
		}
	}
	prompt := "You are a senior growth, SEO and social media consultant. Using ONLY the measured data below, write practical advice. Return JSON only: {\"summary\":\"3 sentences\",\"strengths\":[3 short items],\"improvements\":[5 specific prioritized fixes],\"plan\":[6 steps for the next 30 days],\"ideas\":[5 concrete content, keyword or hook ideas]}.\n\n" + b.String() + "\nExtra context: " + clip(extra, 900)
	txt, err := s.gemini(ctx, prompt)
	if err != nil {
		return nil
	}
	var in AIInsights
	if json.Unmarshal([]byte(cleanJSON(txt)), &in) != nil || in.Summary == "" {
		return nil
	}
	return &in
}
