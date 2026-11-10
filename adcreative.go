package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Ad creative generator: a creative brief, video scripts and finished ad copy for paid social.

type adIn struct {
	Product   string `json:"product"`
	Offer     string `json:"offer"`
	Audience  string `json:"audience"`
	Platform  string `json:"platform"`
	Objective string `json:"objective"`
	Tone      string `json:"tone"`
	Proof     string `json:"proof"`
	Winners   string `json:"winners"`
	Country   string `json:"country"`
	Count     int    `json:"count"`
}

type adScene struct {
	Time      string `json:"time"`
	Visual    string `json:"visual"`
	Voiceover string `json:"voiceover"`
	OnScreen  string `json:"onScreen"`
}

type adCopy struct {
	Primary     string `json:"primary"`
	Headline    string `json:"headline"`
	Description string `json:"description"`
	CTA         string `json:"cta"`
}

type adConcept struct {
	Angle      string    `json:"angle"`
	Hypothesis string    `json:"hypothesis"`
	Hook       string    `json:"hook"`
	Script     []adScene `json:"script"`
	Copy       adCopy    `json:"adCopy"`
	StaticIdea string    `json:"staticIdea"`
}

type adBrief struct {
	Objective   string   `json:"objective"`
	Audience    string   `json:"audience"`
	Insight     string   `json:"insight"`
	KeyMessage  string   `json:"keyMessage"`
	ProofPoints []string `json:"proofPoints"`
	Mandatories []string `json:"mandatories"`
	Avoid       []string `json:"avoid"`
	Deliverable []string `json:"deliverables"`
	Success     []string `json:"successMetrics"`
	TestPlan    string   `json:"testPlan"`
}

type adOut struct {
	Brief    adBrief     `json:"brief"`
	Concepts []adConcept `json:"concepts"`
}

var ctaOK = map[string]bool{"LEARN_MORE": true, "SHOP_NOW": true, "SIGN_UP": true, "CONTACT_US": true, "BOOK_NOW": true, "GET_QUOTE": true}

func (s *server) adCreative(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	uid, err := s.uid(r)
	if err != nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	var in adIn
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil {
		http.Error(w, "bad request", 400)
		return
	}
	in.Product = strings.TrimSpace(in.Product)
	if len([]rune(in.Product)) < 3 {
		http.Error(w, "Describe the product or service you are advertising.", 400)
		return
	}
	if in.Count < 1 || in.Count > 5 {
		in.Count = 3
	}
	if !takeBudget(uid, 25) {
		http.Error(w, "Daily AI limit reached (25). Try again tomorrow.", 429)
		return
	}
	plat := map[string]string{
		"meta":      "Meta (Facebook and Instagram feed and Reels), vertical 9:16 for Reels and Stories, 4:5 for feed",
		"reels":     "Instagram Reels, vertical 9:16, 15-30 seconds, sound on, captions on screen",
		"tiktok":    "TikTok In-Feed ads, vertical 9:16, 15-34 seconds, native creator style, trending-sound friendly",
		"stories":   "Instagram and Facebook Stories, vertical 9:16, 6-15 seconds, tap-forward friendly",
	}[in.Platform]
	if plat == "" {
		plat = "Meta (Facebook and Instagram)"
	}
	prompt := fmt.Sprintf(`You are a senior direct-response paid-social creative strategist for DTC brands. Write a creative brief, video scripts and finished ads that a media buyer can launch today.
Product or service: %s
Offer: %s
Audience: %s
Platform and format: %s
Campaign objective: %s
Tone: %s
Proof points the brand really has: %s
Country or market: %s
Ads that already won for this account (keep what made them work, change the opening and the visual): %s

Rules:
- Never invent statistics, reviews, awards, prices or guarantees. Use only the proof points given; otherwise write placeholders like [add your review count].
- Every concept needs a different angle (for example pain point, social proof, demo, before and after, offer, myth-busting, founder story, comparison).
- The hook is the first 3 seconds: a pattern interrupt or a specific promise, never "Hi guys".
- Scripts are timed scenes with a visual, a spoken line and on-screen text, ending with a clear call to action.
- Finished ad copy: primary text at most 125 characters, headline at most 40, description at most 30, cta one of LEARN_MORE, SHOP_NOW, SIGN_UP, CONTACT_US, BOOK_NOW, GET_QUOTE.
- Each concept has a one-line test hypothesis ("If we ..., then ... because ...").
Return JSON only: {"brief":{"objective":"","audience":"","insight":"the audience insight","keyMessage":"","proofPoints":[],"mandatories":[],"avoid":[],"deliverables":[],"successMetrics":["name the target ROAS, CPA or conversion rate logic, without inventing numbers"],"testPlan":"how to test the concepts, budget split and when to cut"},"concepts":[{"angle":"","hypothesis":"","hook":"","script":[{"time":"0-3s","visual":"","voiceover":"","onScreen":""}],"adCopy":{"primary":"","headline":"","description":"","cta":"SHOP_NOW"},"staticIdea":"one finished static image ad idea with its on-image text"}]}
Write exactly %d concepts.`,
		clip(in.Product, 300), clip(in.Offer, 200), clip(in.Audience, 250), plat, clip(in.Objective, 80), clip(in.Tone, 80), clip(in.Proof, 400), clip(in.Country, 60), clip(in.Winners, 900), in.Count)
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	txt, err := s.gemini(ctx, prompt)
	if err != nil {
		if errors.Is(err, errRate) {
			http.Error(w, "The AI is busy right now (usage limit). Wait a minute and try again.", 429)
			return
		}
		http.Error(w, "The AI could not write the ads. Try again.", 502)
		return
	}
	var out adOut
	if json.Unmarshal([]byte(cleanJSON(txt)), &out) != nil || len(out.Concepts) == 0 {
		http.Error(w, "The AI answer was not usable. Try again.", 502)
		return
	}
	for i := range out.Concepts {
		c := &out.Concepts[i]
		c.Copy.Primary = cut(c.Copy.Primary, 125)
		c.Copy.Headline = cut(c.Copy.Headline, 40)
		c.Copy.Description = cut(c.Copy.Description, 30)
		c.Copy.CTA = strings.ToUpper(strings.TrimSpace(c.Copy.CTA))
		if !ctaOK[c.Copy.CTA] {
			c.Copy.CTA = "LEARN_MORE"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}
