# Blitz AI v3 – one-time setup (all free)

Use the same Google Cloud project as your Firebase project (blitz-ai-59261).

## A. Turn on the Google APIs
1. Open https://console.cloud.google.com/ and pick project **blitz-ai-59261** (top bar).
2. APIs & Services > Library. Search and click **Enable** for each:
   - YouTube Data API v3
   - YouTube Analytics API
   - Google Search Console API
   - Google Analytics Data API
   - Google Analytics Admin API

## B. YouTube API key (for "YouTube (auto)")
1. APIs & Services > Credentials > Create credentials > **API key**. Copy it.
2. Render > blitz-ai > Environment > Add: `YOUTUBE_API_KEY` = the key.

## C. Connect Google (Search Console + GA4 + your YouTube)
1. APIs & Services > OAuth consent screen (Google Auth Platform). Get started: app name Blitz AI, your email, audience **External**.
2. Audience > Test users > add the Google account that owns your channel / Search Console / GA4.
3. Data access > Add or remove scopes > paste these four and save:
   - https://www.googleapis.com/auth/webmasters.readonly
   - https://www.googleapis.com/auth/analytics.readonly
   - https://www.googleapis.com/auth/yt-analytics.readonly
   - https://www.googleapis.com/auth/youtube
4. Credentials > Create credentials > **OAuth client ID** > Web application. Authorized redirect URI:
   `https://blitz-ai-5ow4.onrender.com/auth/google/callback`
   Copy the Client ID and Client secret.
5. Render > Environment > Add:
   - `GOOGLE_CLIENT_ID`
   - `GOOGLE_CLIENT_SECRET`
   - `BASE_URL` = `https://blitz-ai-5ow4.onrender.com`
6. Save; Render redeploys. In the app: tab **Connected Google** > Connect Google.

Notes: while the consent screen is in "Testing", Google expires the connection after about 7 days (just click Reconnect). Your site must be verified in Search Console and your GA4 property must exist under the same Google account.
