# Blitz AI - one-script setup & deploy (all free tiers).
# Run:  powershell -ExecutionPolicy Bypass -File .\deploy.ps1
$ErrorActionPreference = "Continue"
Set-Location $PSScriptRoot
function Step($m)  { Write-Host "`n=== $m ===" -ForegroundColor Cyan }
function Pause2($m){ Read-Host "$m (press Enter when done)" | Out-Null }
function Refresh-Path { $env:Path = [Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [Environment]::GetEnvironmentVariable("Path","User") }
function Need($cmd, $id) {
  if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) {
    Write-Host "Installing $id ..."
    winget install -e --id $id --accept-source-agreements --accept-package-agreements | Out-Null
    Refresh-Path
  }
}

Step "1/6  Installing tools (Git, Go, GitHub CLI, Node)"
Need git Git.Git; Need go GoLang.Go; Need gh GitHub.cli; Need node OpenJS.NodeJS.LTS
if (-not (Get-Command firebase -ErrorAction SilentlyContinue)) { npm install -g firebase-tools; Refresh-Path }

Step "2/6  Gemini API key (free)"
Start-Process "https://aistudio.google.com/app/apikey"
$gkey = (Read-Host "Create a key on the page that opened, then paste it here").Trim()

Step "3/6  Firebase project (free Spark plan)"
firebase login
$fid = "blitz-ai-" + (Get-Random -Minimum 10000 -Maximum 99999)
firebase projects:create $fid --display-name "Blitz AI"
firebase apps:create WEB "Blitz AI" --project $fid
$appId = (firebase apps:list WEB --project $fid --json | ConvertFrom-Json).result[0].appId
$sdk = (firebase apps:sdkconfig WEB $appId --project $fid) -join "`n"
if ($sdk -match 'apiKey"?\s*:\s*"([^"]+)"') { $webKey = $Matches[1] } else { $webKey = Read-Host "Paste the Firebase web apiKey" }
firebase firestore:databases:create "(default)" --location nam5 --project $fid
firebase deploy --only firestore:rules --project $fid

Write-Host "`nEnable Google sign-in: click 'Get started' > Google > Enable > pick your email > Save." -ForegroundColor Yellow
Start-Process "https://console.firebase.google.com/project/$fid/authentication/providers"
Pause2 "Google sign-in enabled"

Write-Host "`nClick 'Generate new private key' > Generate key (it downloads a .json file)." -ForegroundColor Yellow
Start-Process "https://console.firebase.google.com/project/$fid/settings/serviceaccounts/adminsdk"
do {
  Start-Sleep 3
  $key = Get-ChildItem "$HOME\Downloads\$fid-firebase-adminsdk*.json" -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1
} until ($key)
$cred = Get-Content $key.FullName -Raw | ConvertFrom-Json | ConvertTo-Json -Compress
Remove-Item $key.FullName   # key now lives only in render-env.txt (git-ignored)

Step "4/6  GitHub"
gh auth login --web --git-protocol https
if (-not (git config user.email)) { git config user.email "blitz@users.noreply.github.com"; git config user.name "Blitz AI" }
go mod tidy
if (-not (Test-Path .git)) { git init -b main | Out-Null }
@"
FIREBASE_CREDENTIALS=$cred
FIREBASE_PROJECT_ID=$fid
FIREBASE_WEB_API_KEY=$webKey
FIREBASE_AUTH_DOMAIN=$fid.firebaseapp.com
GEMINI_API_KEY=$gkey
"@ | Set-Content render-env.txt -Encoding ascii
git add .
git commit -m "Blitz AI"
gh repo create blitz-ai --public --source . --push
$user = gh api user -q .login

Step "5/6  Render (free web service)"
Write-Host "Render will ask for 5 environment variables. Copy each value from render-env.txt (opening now)." -ForegroundColor Yellow
Start-Process notepad "render-env.txt"
Start-Process "https://dashboard.render.com/blueprint/new?repo=https://github.com/$user/blitz-ai"
Pause2 "Render deploy finished (wait for 'Live')"

Step "6/6  Allow your site to log in"
$site = Read-Host "Paste your Render URL (e.g. https://blitz-ai.onrender.com)"
$host_ = ([uri]$site).Host
Write-Host "Add '$host_' under Authorized domains." -ForegroundColor Yellow
Start-Process "https://console.firebase.google.com/project/$fid/authentication/settings"
Pause2 "Domain added"

Write-Host "`nDone! Open $site" -ForegroundColor Green
Write-Host "Optional: add a custom domain on Cloudflare (CNAME to $host_, proxied) and Render > Custom Domains."
