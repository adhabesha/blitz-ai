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
	{"PUBLISH_SCOPES", "Request posting permissions (type 1 to enable)", "Publishing", "Turn this on only after enabling posting permissions in each developer app, then reconnect each account.", "", false},
	{"CRM_SCOPES", "CRM chat automation permissions (type 1 to enable)", "Facebook", "Adds lead forms, Messenger and Instagram DM/comment permissions to the Facebook sign-in, then reconnect. If you use a Configuration ID, add the same permissions there instead.", "", false},
	{"ADS_MAX_DAILY_BUDGET", "Max daily ad budget per campaign (safety cap)", "Core", "Default 20 in your ad account currency. Raise it only when you are ready to spend more.", "", false},
	{"MEDIA_DIR", "Folder for uploaded media (optional)", "Core", "Where uploaded videos wait for scheduled posting. Default is the server temp folder, which is cleared on restart. Point it at a persistent disk (for example /var/data) so scheduled Instagram and TikTok posts survive restarts.", "", false},
	{"BASE_URL", "Public app URL", "Core", "Your Render address, e.g. https://blitz-ai-5ow4.onrender.com (no slash at the end).", "", false},
	{"CONTACT_EMAIL", "Public contact email (shown on Privacy page)", "Core", "Shown on /privacy, /terms and /data-deletion. Use a business address you are happy to make public.", "", false},
	{"MAIL_FROM", "Email sender (for invitations)", "Email", "For example: Blitz CRM <crm@yourdomain.com>. The domain must be verified with your email provider.", "", false},
	{"RESEND_API_KEY", "Resend API key (easiest)", "Email", "Free at resend.com: create a key and verify your domain. Or use SMTP below instead.", "", true},
	{"SMTP_HOST", "SMTP host (alternative to Resend)", "Email", "For example smtp.gmail.com, smtp.office365.com or your hosting provider's mail server.", "", false},
	{"SMTP_PORT", "SMTP port", "Email", "587 (default) or 465.", "", false},
	{"SMTP_USER", "SMTP username", "Email", "Usually the full email address.", "", false},
	{"SMTP_PASS", "SMTP password", "Email", "For Gmail use an App Password.", "", true},
	{"GEMINI_API_KEY", "Gemini API key", "AI", "From aistudio.google.com/app/apikey. Use a key from a separate project.", "gemini", true},
	{"GEMINI_MODEL", "Gemini model (optional)", "AI", "Leave empty for the default. Change it if Google retires a model.", "", false},
	{"YOUTUBE_API_KEY", "YouTube Data API key", "Google & YouTube", "Google Cloud > Credentials > API key (YouTube Data API v3 enabled).", "youtube", true},
	{"PSI_API_KEY", "PageSpeed Insights key (optional)", "Google & YouTube", "Same kind of key with PageSpeed Insights API enabled. Makes speed tests reliable.", "psi", true},
	{"GOOGLE_CLIENT_ID", "Google OAuth client ID", "Google & YouTube", "Google Cloud > Credentials > OAuth client (Web application).", "google", false},
	{"GOOGLE_CLIENT_SECRET", "Google OAuth client secret", "Google & YouTube", "Shown once when you create the OAuth client.", "", true},
	{"INSTAGRAM_CONFIG_ID", "Instagram login configuration ID", "Instagram", "Meta app > Instagram > API setup with Unified login > Configuration ID. Instagram also uses the Facebook app ID and secret below.", "instagram", false},
	{"INSTAGRAM_USER_ID", "Instagram account ID (only if sign-in cannot find it)", "Instagram", "Meta Business settings > Accounts > Instagram accounts > click your account > ID.", "", false},
	{"FACEBOOK_APP_ID", "Facebook app ID", "Facebook", "Meta app > Settings > Basic.", "facebook", false},
	{"FACEBOOK_APP_SECRET", "Facebook app secret", "Facebook", "Meta app > Settings > Basic.", "", true},
	{"FACEBOOK_CONFIG_ID", "Facebook login configuration ID (optional)", "Facebook", "Meta app > Facebook Login for Business > Configurations. Leave empty to use classic permissions.", "", false},
	{"FACEBOOK_PAGE", "Facebook Page username to post to", "Facebook", "For example sikettc. Leave empty to use the first Page you manage.", "", false},
	{"X_CLIENT_ID", "X client ID", "X (Twitter)", "developer.x.com > your project > app > Keys and tokens > OAuth 2.0 Client ID. Reading data needs a paid X API plan.", "", false},
	{"X_CLIENT_SECRET", "X client secret", "X (Twitter)", "Same page: OAuth 2.0 Client Secret. Set the callback URL shown above.", "", true},
	{"LINKEDIN_CLIENT_ID", "LinkedIn client ID", "LinkedIn", "linkedin.com/developers > your app > Auth.", "", false},
	{"LINKEDIN_CLIENT_SECRET", "LinkedIn client secret", "LinkedIn", "Same page. Add the redirect URL shown above.", "", true},
	{"LINKEDIN_PARTNER", "LinkedIn partner access (type 1 after approval)", "LinkedIn", "Set to 1 only once LinkedIn approves your app for the Marketing Developer Platform (page analytics). Then reconnect.", "", false},
	{"PINTEREST_APP_ID", "Pinterest app ID", "Pinterest", "developers.pinterest.com > your app.", "", false},
	{"PINTEREST_APP_SECRET", "Pinterest app secret", "Pinterest", "Same page. Add the redirect URL shown above.", "", true},
	{"TIKTOK_SCOPES", "TikTok scopes (optional)", "TikTok", "Comma-separated. Leave empty for the default. For testing, try only user.info.basic.", "", false},
	{"TIKTOK_CLIENT_KEY", "TikTok client key", "TikTok", "developers.tiktok.com > your app.", "tiktok", false},
	{"TIKTOK_CLIENT_SECRET", "TikTok client secret", "TikTok", "developers.tiktok.com > your app.", "", true},
	{"TIKTOK_AUDITED", "TikTok app audited (set 1 only after approval)", "TikTok", "Leave empty while the app is in sandbox/unaudited: posts are sent as private. Set to 1 once TikTok approves your Content Posting API audit to allow public posts.", "", false},
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
