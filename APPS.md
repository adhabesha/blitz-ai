# Blitz AI desktop (.exe) and Android (.apk)

Both apps open your live Blitz AI site (sign in, onboarding, dashboard, admin), so every website update reaches them instantly.

## Get the files (free, built by GitHub)
1. Push this project to GitHub (update.ps1 does it).
2. GitHub repo > **Actions** > **Build desktop and mobile apps** > **Run workflow**.
3. Wait about 5-10 minutes. Open the finished run and download **blitz-ai-windows** (the .exe files) and **blitz-ai-android** (app-debug.apk) from the Artifacts section.

## Install
- Windows: run Blitz-AI-Setup-*.exe (installs) or Blitz-AI-Portable-*.exe (no install).
- Android: copy app-debug.apk to your phone, open it, allow "Install unknown apps" for your file manager, install.

## One-time switch in Firebase
Firebase console > Authentication > Sign-in method > enable **Email/Password** (Google sign-in is hidden inside the apps because Google blocks it in embedded windows).

## Admin console
Open https://YOUR-APP.onrender.com/admin (or Ctrl+Shift+A in the desktop app). Add your email to ADMIN_EMAILS in Render once, and use an email that is verified. Set STATE_SECRET in Render to a long random text once and never change it (it protects the keys saved in admin).
