# Blitz AI – free-tier SaaS (Go + Firebase + Gemini + Render)
Easiest: run `powershell -ExecutionPolicy Bypass -File .\deploy.ps1` and follow the prompts.

Stack: Go app on Render free, Firebase Auth + Firestore (Spark, no expiry), Gemini API free tier, GitHub, optional Cloudflare for a custom domain (CNAME to your onrender.com URL, proxied).

Notes
- Gemini free tier is rate-limited, not unlimited. The app caps each user at 5 analyses/day. Change `GEMINI_MODEL` in Render env if a model ID is retired (copy IDs from AI Studio).
- Free-tier Gemini inputs may be used by Google to improve models; don't send private data.
- Render free sleeps after ~15 min idle; ping /healthz with UptimeRobot.
