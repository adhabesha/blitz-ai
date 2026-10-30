# Connect Instagram, Facebook and TikTok (owner setup, free)

Redirect URLs to register (use your own Render address):
- Instagram: https://blitz-ai-5ow4.onrender.com/auth/instagram/callback
- Facebook:  https://blitz-ai-5ow4.onrender.com/auth/facebook/callback
- TikTok:    https://blitz-ai-5ow4.onrender.com/auth/tiktok/callback

`BASE_URL` must already be set in Render (`https://blitz-ai-5ow4.onrender.com`).

Developer consoles change often. If a screen looks different, send a screenshot.

## Instagram (Business or Creator account only)
1. https://developers.facebook.com/apps > Create app (type: Business). Add the product **Instagram** > **API setup with Instagram login**.
2. In "Business login settings" add the Instagram redirect URL above.
3. Copy the **Instagram app ID** and **Instagram app secret** shown on that page (they are NOT the Facebook app ID/secret).
4. App roles > Roles: add your Instagram account as an **Instagram Tester**, then accept the invite inside Instagram (Settings > Apps and websites > Tester invites).
5. Render > Environment: `INSTAGRAM_APP_ID`, `INSTAGRAM_APP_SECRET`.

## Facebook (Pages you manage)
1. Same Meta app > add **Facebook Login** (or Facebook Login for Business). Add the Facebook redirect URL as a Valid OAuth Redirect URI.
2. App settings > Basic: copy **App ID** and **App secret**.
3. Render: `FACEBOOK_APP_ID`, `FACEBOOK_APP_SECRET`.

## TikTok (your own account)
1. https://developers.tiktok.com > Manage apps > Connect an app. Add **Login Kit** and **Display API** with scopes user.info.basic, user.info.profile, user.info.stats, video.list.
2. Add the TikTok redirect URL. In **Sandbox**, add your TikTok account as a target user.
3. Copy the **Client key** and **Client secret**.
4. Render: `TIKTOK_CLIENT_KEY`, `TIKTOK_CLIENT_SECRET`.

## Important
- While apps are in development/sandbox mode only accounts you add as testers can connect. Letting any customer connect needs each platform's app review (Meta, TikTok), which can take days to weeks.
- X and LinkedIn can't be connected for free; use the "Other social (manual)" tab for them.
