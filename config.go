package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log"
	"os"
	"sync"
	"time"
)

var (
	cfgMu   sync.RWMutex
	cfgVals = map[string]string{}
)

// env returns a value set in the admin console, falling back to the Render environment.
func env(k string) string {
	cfgMu.RLock()
	v := cfgVals[k]
	cfgMu.RUnlock()
	if v != "" {
		return v
	}
	return os.Getenv(k)
}

func geminiModel() string {
	if m := env("GEMINI_MODEL"); m != "" {
		return m
	}
	return "gemini-3.5-flash-lite"
}

type cfgItem struct {
	Key, Label, Group, Help, Test string
	Secret                        bool
}

var adminItems = []cfgItem{
	{"BASE_URL", "Public app URL", "Core", "Your Render address, e.g. https://blitz-ai-5ow4.onrender.com (no slash at the end).", "", false},
	{"GEMINI_API_KEY", "Gemini API key", "AI", "From aistudio.google.com/app/apikey. Use a key from a separate project.", "gemini", true},
	{"GEMINI_MODEL", "Gemini model (optional)", "AI", "Leave empty for the default. Change it if Google retires a model.", "", false},
	{"YOUTUBE_API_KEY", "YouTube Data API key", "Google & YouTube", "Google Cloud > Credentials > API key (YouTube Data API v3 enabled).", "youtube", true},
	{"PSI_API_KEY", "PageSpeed Insights key (optional)", "Google & YouTube", "Same kind of key with PageSpeed Insights API enabled. Makes speed tests reliable.", "psi", true},
	{"GOOGLE_CLIENT_ID", "Google OAuth client ID", "Google & YouTube", "Google Cloud > Credentials > OAuth client (Web application).", "google", false},
	{"GOOGLE_CLIENT_SECRET", "Google OAuth client secret", "Google & YouTube", "Shown once when you create the OAuth client.", "", true},
	{"INSTAGRAM_APP_ID", "Instagram app ID", "Instagram", "Meta app > Instagram > API setup with Instagram login (not the Facebook app ID).", "instagram", false},
	{"INSTAGRAM_APP_SECRET", "Instagram app secret", "Instagram", "On the same Instagram API setup page.", "", true},
	{"FACEBOOK_APP_ID", "Facebook app ID", "Facebook", "Meta app > Settings > Basic.", "facebook", false},
	{"FACEBOOK_APP_SECRET", "Facebook app secret", "Facebook", "Meta app > Settings > Basic.", "", true},
	{"TIKTOK_CLIENT_KEY", "TikTok client key", "TikTok", "developers.tiktok.com > your app.", "tiktok", false},
	{"TIKTOK_CLIENT_SECRET", "TikTok client secret", "TikTok", "developers.tiktok.com > your app.", "", true},
}

func cfgKey() []byte {
	seed := os.Getenv("STATE_SECRET")
	if seed == "" {
		seed = os.Getenv("FIREBASE_CREDENTIALS")
	}
	sum := sha256.Sum256([]byte("blitz-config|" + seed))
	return sum[:]
}

func seal(plain string) (string, error) {
	block, err := aes.NewCipher(cfgKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func unseal(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(cfgKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return "", errors.New("short ciphertext")
	}
	pt, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	return string(pt), err
}

func (s *server) loadConfig(ctx context.Context) {
	doc, err := s.db.Collection("config").Doc("integrations").Get(ctx)
	if err != nil {
		return
	}
	m, _ := doc.Data()["v"].(map[string]any)
	out := map[string]string{}
	for k, x := range m {
		enc, _ := x.(string)
		if enc == "" {
			continue
		}
		if v, err := unseal(enc); err == nil {
			out[k] = v
		} else {
			log.Println("config: cannot decrypt", k, "(did STATE_SECRET change?)")
		}
	}
	cfgMu.Lock()
	cfgVals = out
	cfgMu.Unlock()
}

func (s *server) setConfig(ctx context.Context, key, value string) error {
	ref := s.db.Collection("config").Doc("integrations")
	cur := map[string]any{}
	if doc, err := ref.Get(ctx); err == nil {
		if m, ok := doc.Data()["v"].(map[string]any); ok {
			cur = m
		}
	}
	if value == "" {
		delete(cur, key)
	} else {
		enc, err := seal(value)
		if err != nil {
			return err
		}
		cur[key] = enc
	}
	if _, err := ref.Set(ctx, map[string]any{"v": cur, "updatedAt": time.Now()}); err != nil {
		return err
	}
	s.loadConfig(ctx)
	return nil
}
