package main

// Multi-provider AI router.
//
// Every text prompt goes to the first provider that has a key and is not cooling down. When a provider hits its
// free limit (429), rejects the key or is out of credit, it is paused for a while and the next one is tried.
// Default order: free tiers first (Gemini, Groq, Cerebras, OpenRouter free models, Mistral), then the optional paid
// accounts (Claude, Grok, OpenAI). Change it with AI_ORDER (comma separated ids), e.g. "claude,gemini" to prefer a paid key.
// Prompts that include images, and live Google Search grounding, stay on Gemini because only Gemini offers them here.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type aiProv struct {
	ID, Name, KeyEnv, ModelEnv, DefModel, Base string
	Free                                       bool
	Kind                                       string // gemini | openai | anthropic
	JSONMode                                   bool   // provider accepts response_format json_object
}

var aiProvs = []aiProv{
	{ID: "gemini", Name: "Gemini", KeyEnv: "GEMINI_API_KEY", ModelEnv: "GEMINI_MODEL", Free: true, Kind: "gemini"},
	{ID: "groq", Name: "Groq", KeyEnv: "GROQ_API_KEY", ModelEnv: "GROQ_MODEL", DefModel: "llama-3.3-70b-versatile", Base: "https://api.groq.com/openai/v1", Free: true, Kind: "openai", JSONMode: true},
	{ID: "cerebras", Name: "Cerebras", KeyEnv: "CEREBRAS_API_KEY", ModelEnv: "CEREBRAS_MODEL", DefModel: "llama-3.3-70b", Base: "https://api.cerebras.ai/v1", Free: true, Kind: "openai", JSONMode: true},
	{ID: "openrouter", Name: "OpenRouter (free models)", KeyEnv: "OPENROUTER_API_KEY", ModelEnv: "OPENROUTER_MODEL", DefModel: "meta-llama/llama-3.3-70b-instruct:free", Base: "https://openrouter.ai/api/v1", Free: true, Kind: "openai"},
	{ID: "mistral", Name: "Mistral", KeyEnv: "MISTRAL_API_KEY", ModelEnv: "MISTRAL_MODEL", DefModel: "mistral-small-latest", Base: "https://api.mistral.ai/v1", Free: true, Kind: "openai", JSONMode: true},
	{ID: "claude", Name: "Claude (paid)", KeyEnv: "ANTHROPIC_API_KEY", ModelEnv: "CLAUDE_MODEL", DefModel: "claude-haiku-5-5", Kind: "anthropic"},
	{ID: "grok", Name: "Grok (paid)", KeyEnv: "XAI_API_KEY", ModelEnv: "GROK_MODEL", DefModel: "grok-4", Base: "https://api.x.ai/v1", Kind: "openai", JSONMode: true},
	{ID: "openai", Name: "OpenAI (paid)", KeyEnv: "OPENAI_API_KEY", ModelEnv: "OPENAI_MODEL", DefModel: "gpt-4o-mini", Base: "https://api.openai.com/v1", Kind: "openai", JSONMode: true},
}

var (
	aiMu    sync.Mutex
	aiPause = map[string]time.Time{} // provider id -> paused until
	aiLastP string                   // provider that answered last
	aiHTTP  = &http.Client{Timeout: 45 * time.Second}
)

func (p aiProv) key() string { return strings.TrimSpace(env(p.KeyEnv)) }

func (p aiProv) model() string {
	if m := strings.TrimSpace(env(p.ModelEnv)); m != "" {
		return m
	}
	if p.ID == "gemini" {
		return geminiModel()
	}
	return p.DefModel
}

// aiOrder lists the providers that have a key, in the order they will be tried.
func aiOrder() []aiProv {
	var ids []string
	if o := strings.TrimSpace(env("AI_ORDER")); o != "" {
		for _, x := range strings.Split(o, ",") {
			ids = append(ids, strings.ToLower(strings.TrimSpace(x)))
		}
	}
	for _, p := range aiProvs { // anything not named in AI_ORDER follows in the default order
		ids = append(ids, p.ID)
	}
	seen := map[string]bool{}
	var out []aiProv
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, p := range aiProvs {
			if p.ID == id && p.key() != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// aiReady reports whether any AI provider has a key.
func aiReady() bool { return len(aiOrder()) > 0 }

// aiStatus is shown in the admin console.
func aiStatus() string {
	o := aiOrder()
	if len(o) == 0 {
		return "No AI key set yet."
	}
	var n []string
	for _, p := range o {
		n = append(n, p.Name)
	}
	return "Order: " + strings.Join(n, " → ")
}

func aiPaused(id string) bool {
	aiMu.Lock()
	defer aiMu.Unlock()
	return time.Now().Before(aiPause[id])
}

func aiPauseFor(id string, d time.Duration) {
	aiMu.Lock()
	aiPause[id] = time.Now().Add(d)
	aiMu.Unlock()
}

// aiText sends one text prompt through the provider chain and returns JSON text.
func (s *server) aiText(ctx context.Context, prompt string) (string, error) {
	order := aiOrder()
	if len(order) == 0 {
		return "", errors.New("AI is not set up (add a Gemini key, or Groq, Cerebras, OpenRouter, Mistral, Claude, Grok or OpenAI)")
	}
	var lastErr error
	rateOnly := true
	tried := 0
	// first pass skips paused providers; if everything is paused, try them anyway once
	for pass := 0; pass < 2 && tried == 0; pass++ {
		for _, p := range order {
			if pass == 0 && aiPaused(p.ID) {
				continue
			}
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			tried++
			var txt string
			var err error
			switch p.Kind {
			case "gemini":
				txt, err = s.geminiCall(ctx, []any{map[string]string{"text": prompt}})
			case "anthropic":
				txt, err = anthropicCall(ctx, p, prompt)
			default:
				txt, err = openaiCall(ctx, p, prompt)
			}
			if err == nil && strings.TrimSpace(txt) != "" {
				aiMu.Lock()
				aiLastP = p.ID
				aiMu.Unlock()
				if p.Kind != "gemini" {
					txt = jsonOnly(txt)
				}
				return txt, nil
			}
			if err == nil {
				err = errors.New("empty reply")
			}
			lastErr = fmt.Errorf("%s: %w", p.Name, err)
			switch {
			case errors.Is(err, errRate):
				aiPauseFor(p.ID, 90*time.Second)
			case errors.Is(err, errAuth):
				rateOnly = false
				aiPauseFor(p.ID, 15*time.Minute)
			default:
				rateOnly = false
				aiPauseFor(p.ID, 30*time.Second)
			}
		}
	}
	if lastErr == nil {
		return "", errRate
	}
	if rateOnly {
		return "", errRate
	}
	return "", lastErr
}

var errAuth = errors.New("key rejected or out of credit")

func statusErr(code int, body []byte) error {
	switch {
	case code == http.StatusTooManyRequests:
		return errRate
	case code == 401 || code == 402 || code == 403:
		return fmt.Errorf("%w (status %d)", errAuth, code)
	}
	m := strings.Join(strings.Fields(string(body)), " ")
	if len(m) > 160 {
		m = m[:160]
	}
	return fmt.Errorf("status %d %s", code, m)
}

func openaiCall(ctx context.Context, p aiProv, prompt string) (string, error) {
	req := map[string]any{
		"model":    p.model(),
		"messages": []any{map[string]string{"role": "user", "content": prompt + "\n\nReply with valid JSON only, no commentary."}},
	}
	if p.JSONMode {
		req["response_format"] = map[string]string{"type": "json_object"}
	}
	b, _ := json.Marshal(req)
	hr, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.Base+"/chat/completions", bytes.NewReader(b))
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Authorization", "Bearer "+p.key())
	resp, err := aiHTTP.Do(hr)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", statusErr(resp.StatusCode, raw)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &out) != nil || len(out.Choices) == 0 {
		return "", errors.New("empty reply")
	}
	return out.Choices[0].Message.Content, nil
}

func anthropicCall(ctx context.Context, p aiProv, prompt string) (string, error) {
	b, _ := json.Marshal(map[string]any{
		"model":      p.model(),
		"max_tokens": 4096,
		"messages":   []any{map[string]string{"role": "user", "content": prompt + "\n\nReply with valid JSON only, no commentary."}},
	})
	hr, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(b))
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("x-api-key", p.key())
	hr.Header.Set("anthropic-version", "2023-06-01")
	resp, err := aiHTTP.Do(hr)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 529 { // overloaded
			return "", errRate
		}
		return "", statusErr(resp.StatusCode, raw)
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return "", errors.New("empty reply")
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String(), nil
}

// jsonOnly strips code fences and chatter so callers always receive bare JSON (object or array).
func jsonOnly(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return s
	}
	i, j := strings.IndexAny(s, "{["), strings.LastIndexAny(s, "}]")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}

// aiTest runs one prompt against one named provider (for the admin "Test" buttons).
func (s *server) aiTest(ctx context.Context, id string) (bool, string) {
	for _, p := range aiProvs {
		if p.ID != id {
			continue
		}
		if p.key() == "" {
			return false, "No " + p.Name + " key set."
		}
		var err error
		switch p.Kind {
		case "gemini":
			_, err = s.geminiCall(ctx, []any{map[string]string{"text": `Reply with the JSON {"ok":true}`}})
		case "anthropic":
			_, err = anthropicCall(ctx, p, `Reply with the JSON {"ok":true}`)
		default:
			_, err = openaiCall(ctx, p, `Reply with the JSON {"ok":true}`)
		}
		if err != nil {
			return false, p.Name + " call failed: " + trunc(err.Error(), 140) + " (check the key and the model name)"
		}
		return true, p.Name + " works with model " + p.model() + ". " + aiStatus()
	}
	return false, "Unknown AI provider."
}
