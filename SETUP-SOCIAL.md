# Connect Instagram, Facebook and TikTok (owner setup, free)

Redirect URLs to register (use your own Render address):
- Instagram: https://blitz-ai-5ow4.onrender.com/auth/instagram/callback
- Facebook:  https://blitz-ai-5ow4.onrender.com/auth/facebook/callback
- TikTok:    https://blitz-ai-5ow4.onrender.com/auth/tiktok/callback

`BASE_URL` must already be set in Render (`https://blitz-ai-5ow4.onrender.com`).

Developer consoles change often. If a screen looks different, send a screenshot.

## Facebook Page + Instagram (Meta, one app, Facebook Login for Business)
New Meta apps cannot use the old "Instagram login", so Blitz AI signs in through Facebook and reads Instagram through the Facebook-linked account.
1. In your Meta app: Instagram > API setup with Unified login > Create another configuration. Include the assets Pages and Instagram accounts, and permissions pages_show_list, pages_read_engagement, pages_manage_posts, read_insights, instagram_basic, instagram_content_publish, instagram_manage_insights.
2. Select the configuration and save both redirect URLs: .../auth/instagram/callback and .../auth/facebook/callback.
3. Copy the Configuration ID, and the App ID and App secret from App settings > Basic.
4. Your Instagram must be a Business or Creator account linked to the Facebook Page (Page > Settings > Linked accounts > Instagram).
5. In the admin console set: Facebook app ID and secret, Instagram configuration ID, Facebook configuration ID (same ID), Facebook Page (sikettc).

## TikTok (your own account)
1. https://developers.tiktok.com > Manage apps > Connect an app. Add **Login Kit** and **Display API** with scopes user.info.basic, user.info.profile, user.info.stats, video.list.
2. Add the TikTok redirect URL. In **Sandbox**, add your TikTok account as a target user.
3. Copy the **Client key** and **Client secret**.
4. Render: `TIKTOK_CLIENT_KEY`, `TIKTOK_CLIENT_SECRET`.

## Important
- While apps are in development/sandbox mode only accounts you add as testers can connect. Letting any customer connect needs each platform's app review (Meta, TikTok), which can take days to weeks.
- X and LinkedIn can't be connected for free; use the "Other social (manual)" tab for them.

## Turning on posting (Publish to all)
Reading and posting use different permissions. Do this once per platform, then in the admin console set **Request posting permissions** to `1`, and disconnect and reconnect each account in Connected social.
- **Instagram:** Meta app > Instagram > API setup > add the permission `instagram_business_content_publish`. The account must be Business or Creator, and added as an Instagram Tester.
- **Facebook:** Meta app > add the permission `pages_manage_posts` (with `pages_show_list`, `pages_read_engagement`). Your Facebook login must be an admin of the Page, and added to the app as an admin/tester.
- **TikTok:** developers.tiktok.com > your app > add the product **Content Posting API** and enable Direct Post, scope `video.publish` (and `video.upload`). In Sandbox keep your TikTok account as a target user. Posts from unaudited apps stay private.
- **YouTube:** nothing extra. Uploads from a new Google project may stay private until Google audits it.

## When a connection fails
The dashboard now shows the platform's own reason after "Connection failed". Common ones:
- `redirect_uri` / "URL blocked" (Meta): add the exact /auth/instagram/callback and /auth/facebook/callback URLs under Facebook Login for Business > Settings > Valid OAuth Redirect URIs.
- `scope` / `scope_not_authorized` (TikTok): a scope is requested that the TikTok app doesn't have. Keep "Request posting permissions" at 0 until Content Posting API is added and approved.
- `redirect_uri` (TikTok): the exact /auth/tiktok/callback URL must be registered under Login Kit.
- Meta/TikTok in development mode: your account must be added as a tester / sandbox target user.
- A field in the admin console showing source "admin" overrides the Render value of the same name.
- Facebook now reuses the Instagram configuration ID if no separate Facebook one is set.
