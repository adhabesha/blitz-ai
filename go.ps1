# ficx - ONE script: updates the code, deploys to Render and switches on email sign-in. Add -BuildApps to also build the .exe and .apk.
param([string]$Target = "$HOME\Downloads\blitz-ai\blitz-ai", [switch]$BuildApps, [switch]$Connect)
$ErrorActionPreference = "Continue"
$ProgressPreference = "SilentlyContinue"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$ServiceId       = "srv-dbgvfhmtlv9c73dg5aqg"
$FirebaseProject = "blitz-ai-59261"
$Site            = "https://blitz-ai-5ow4.onrender.com"
$Repo            = "adhabesha/blitz-ai"
$Out             = "$HOME\Downloads\Blitz-AI-Apps"
$Results         = @()

function Step($n, $m) { Write-Host ""; Write-Host "[$n] $m" -ForegroundColor Cyan }
function Ok($m)   { Write-Host "   OK  $m" -ForegroundColor Green }
function Warn($m) { Write-Host "   !!  $m" -ForegroundColor Yellow }
function Bad($m)  { Write-Host "   XX  $m" -ForegroundColor Red }

# ---------------------------------------------------------------- 1. copy files
Step "1/6" "Updating your project files"
if (-not (Test-Path "$Target\go.mod")) {
  $found = Get-ChildItem "$HOME\Downloads" -Filter go.mod -Recurse -Depth 3 -ErrorAction SilentlyContinue | Where-Object { $_.DirectoryName -notmatch "node_modules|blitz-v" } | Select-Object -First 1
  if ($found) { $Target = $found.DirectoryName } else { $Target = Read-Host "Path of your blitz-ai project folder (the one with go.mod)" }
}
Copy-Item "$PSScriptRoot\*.go" $Target -Force
foreach ($d in @("templates", "static", "desktop", "mobile", ".github")) { Copy-Item (Join-Path $PSScriptRoot $d) $Target -Recurse -Force }
Copy-Item "$PSScriptRoot\*.md" $Target -Force
Set-Location $Target
Ok "Files copied to $Target"

# ---------------------------------------------------------------- 2. build check
Step "2/6" "Checking the server code builds"
go mod tidy 2>&1 | Out-Null
$b = go build -o check.exe . 2>&1
if ($LASTEXITCODE -ne 0) {
  Bad "The Go build failed. Nothing was published."
  $b | Out-File "$HOME\Downloads\blitz-build-error.txt"
  $b | ForEach-Object { Write-Host $_ }
  Write-Host "Send the text above to Claude (also saved in Downloads\blitz-build-error.txt)." -ForegroundColor Yellow
  exit 1
}
Remove-Item check.exe -ErrorAction SilentlyContinue
Ok "Build passed"

# ---------------------------------------------------------------- 3. push to GitHub
Step "3/6" "Sending the code to GitHub"
$st = gh auth status 2>&1 | Out-String
if ($st -notmatch "workflow") {
  Warn "One-time: GitHub must allow the build workflow. Copy the code it shows into the browser page and approve."
  gh auth refresh -h github.com -s workflow
}
git add -A
git commit -m "ficx v12" 2>&1 | Out-Null
git push 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
  git push
  if ($LASTEXITCODE -ne 0) { Bad "Push failed. Check your internet (or use a phone hotspot) and run the same command again."; exit 1 }
}
Ok "Code is on GitHub"

# ---------------------------------------------------------------- 4. Render
Step "4/6" "Deploying to Render"
try {
  $keyFile = Join-Path $env:USERPROFILE ".blitz-render.key"
  $mailFile = Join-Path $env:USERPROFILE ".blitz-admin-email.txt"
  $rk = ""
  if (Test-Path $keyFile) {
    try {
      $sec = Get-Content $keyFile | ConvertTo-SecureString
      $rk = [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec))
    } catch { $rk = "" }
  }
  if (-not $rk) {
    Start-Process "https://dashboard.render.com/u/settings#api-keys"
    Warn "One-time: in the page that opened click 'Create API Key', name it blitz, copy the key."
    $rk = (Read-Host "Paste the Render API key here").Trim()
    if ($rk) { ConvertTo-SecureString $rk -AsPlainText -Force | ConvertFrom-SecureString | Set-Content $keyFile }
  }
  $mail = ""
  if (Test-Path $mailFile) { $mail = (Get-Content $mailFile -Raw).Trim() }
  if (-not $mail) {
    $mail = (Read-Host "Your admin email (the one you sign in with)").Trim()
    if ($mail) { Set-Content $mailFile $mail }
  }
  $H = @{ Authorization = "Bearer $rk"; Accept = "application/json"; "Content-Type" = "application/json" }
  $base = "https://api.render.com/v1/services/$ServiceId"
  $have = @{}
  $existing = Invoke-RestMethod -Uri "$base/env-vars?limit=100" -Headers $H
  foreach ($e in $existing) { $have[$e.envVar.key] = $true }
  if ($mail) { Invoke-RestMethod -Method Put -Uri "$base/env-vars/ADMIN_EMAILS" -Headers $H -Body (@{ value = $mail } | ConvertTo-Json) | Out-Null }
  if (-not $have["BASE_URL"]) { Invoke-RestMethod -Method Put -Uri "$base/env-vars/BASE_URL" -Headers $H -Body (@{ value = $Site } | ConvertTo-Json) | Out-Null }
  if (-not $have["STATE_SECRET"]) {
    $chars = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
    $secret = -join (1..48 | ForEach-Object { $chars[(Get-Random -Maximum $chars.Length)] })
    Invoke-RestMethod -Method Put -Uri "$base/env-vars/STATE_SECRET" -Headers $H -Body (@{ value = $secret } | ConvertTo-Json) | Out-Null
  }
  $ans = "n"
  if (-not $Connect) { $ans = Read-Host "Have you already added the posting permissions in your Meta and TikTok developer apps? (y/n)" }
  if ($ans -match "^[yY]") {
    Invoke-RestMethod -Method Put -Uri "$base/env-vars/PUBLISH_SCOPES" -Headers $H -Body (@{ value = "1" } | ConvertTo-Json) | Out-Null
    Ok "Posting permissions switched on. Now reconnect Instagram, Facebook and TikTok in Connected social."
  } elseif (-not $Connect) {
    Warn "Posting is not switched on yet. Add these permissions, then run this same command again and answer y:"
    Write-Host "      Instagram: instagram_business_content_publish   Facebook: pages_manage_posts   TikTok: Content Posting API (video.publish)"
    Start-Process "https://developers.facebook.com/apps"
    Start-Process "https://developers.tiktok.com/apps"
  }
  Ok "Render settings saved"
  Start-Sleep 5
  Invoke-RestMethod -Method Post -Uri "$base/deploys" -Headers $H -Body '{"clearCache":"do_not_clear"}' | Out-Null
  Write-Host "   Deploying (takes 2-5 minutes) " -NoNewline
  $deadline = (Get-Date).AddMinutes(12)
  $status = ""
  while ((Get-Date) -lt $deadline) {
    Start-Sleep 10
    Write-Host "." -NoNewline
    $d = Invoke-RestMethod -Uri "$base/deploys?limit=1" -Headers $H
    $status = @($d)[0].deploy.status
    if (@("live", "build_failed", "update_failed", "canceled", "pre_deploy_failed") -contains $status) { break }
  }
  Write-Host ""
  if ($status -eq "live") { Ok "Website is live: $Site"; $Results += "Website: $Site/app" }
  else { Bad "Render finished with status '$status'. Open Render > blitz-ai > Logs and send me the last 20 lines." }
} catch {
  Bad "Render step failed: $($_.Exception.Message)"
  Warn "Fix: delete the file $HOME\.blitz-render.key and run the same command again with a fresh Render API key."
}

# ---------------------------------------------------------------- 4b. connect social accounts (wizard)
if ($Connect -and $base -and $H) {
  Step "4b" "Connecting Instagram, Facebook and TikTok"
  function Put-Env($k, $v) { Invoke-RestMethod -Method Put -Uri "$base/env-vars/$k" -Headers $H -Body (@{ value = $v } | ConvertTo-Json) | Out-Null }
  function Read-Secret($label) { $s = Read-Host $label -AsSecureString; return [Runtime.InteropServices.Marshal]::PtrToStringAuto([Runtime.InteropServices.Marshal]::SecureStringToBSTR($s)) }
  function Save-Keys($name, $idVar, $secVar, $idLabel, $secLabel) {
    $id = (Read-Host "   Paste the $idLabel").Trim()
    $sec = (Read-Secret "   Paste the $secLabel").Trim()
    if ($id -and $sec) { Put-Env $idVar $id; Put-Env $secVar $sec; Ok "$name keys saved" } else { Warn "$name skipped (a value was empty)" }
  }
  Write-Host "   Accounts: Instagram @aceaddiss | Facebook Page sikettc | TikTok @techetx"
  Write-Host "   Do the numbered steps in the page that opens. Press Enter here when each part is done." -ForegroundColor Yellow

  Write-Host ""; Write-Host "   --- META (one app, one configuration for Facebook Page + Instagram) ---" -ForegroundColor Cyan
  Write-Host "   Your Meta app already exists. In it:"
  Write-Host "   1. Instagram > API setup with Unified login > 'Create another configuration'. Include BOTH assets: Pages and Instagram accounts."
  Write-Host "      Permissions: pages_show_list, pages_read_engagement, pages_manage_posts, read_insights,"
  Write-Host "      instagram_basic, instagram_content_publish, instagram_manage_insights (add what the list offers)."
  Write-Host "   2. Select that configuration and register BOTH redirect URLs (save each):"
  Write-Host "        $Site/auth/instagram/callback"
  Write-Host "        $Site/auth/facebook/callback"
  Write-Host "   3. Copy the new Configuration ID (shown on the configuration card)."
  Write-Host "   4. App settings > Basic: copy the App ID and App secret (Show)."
  Write-Host "   5. Link @aceaddiss to the sikettc Page: on Facebook open the Page > Settings > Linked accounts > Instagram > Connect."
  Write-Host "      @aceaddiss must be a Business or Creator account (Instagram > Settings > Account type and tools)."
  Set-Clipboard -Value "$Site/auth/instagram/callback"
  Start-Process "https://developers.facebook.com/apps"
  $cfg = (Read-Host "   Paste the Configuration ID").Trim()
  $aid = (Read-Host "   Paste the App ID (Enter keeps 1045705618555587)").Trim()
  if (-not $aid) { $aid = "1045705618555587" }
  $asec = (Read-Secret "   Paste the App secret").Trim()
  if ($cfg -and $asec) {
    Put-Env "FACEBOOK_APP_ID" $aid; Put-Env "FACEBOOK_APP_SECRET" $asec
    Put-Env "INSTAGRAM_CONFIG_ID" $cfg; Put-Env "FACEBOOK_CONFIG_ID" $cfg
    Put-Env "FACEBOOK_PAGE" "sikettc"
    Ok "Meta settings saved"
  } else { Warn "Meta skipped (a value was empty)" }

  Write-Host ""; Write-Host "   --- TIKTOK ---" -ForegroundColor Cyan
  Write-Host "   1. Connect an app: name ficx, add any icon. Add products: Login Kit, Content Posting API (turn on Direct Post), Display API."
  Write-Host "   2. Login Kit > Redirect URI: paste (already on your clipboard)."
  Write-Host "   3. Scopes: user.info.basic, user.info.profile, user.info.stats, video.list, video.publish, video.upload."
  Write-Host "   4. Sandbox: add @techetx as a target user and log in with it."
  Write-Host "   5. Copy the Client key and Client secret."
  Set-Clipboard -Value "$Site/auth/tiktok/callback"
  Start-Process "https://developers.tiktok.com/apps"
  Save-Keys "TikTok" "TIKTOK_CLIENT_KEY" "TIKTOK_CLIENT_SECRET" "TikTok Client key" "TikTok Client secret"

  $go = Read-Host "   Did the configuration include pages_manage_posts and instagram_content_publish? (y/n)"
  if ($go -match "^[yY]") { Put-Env "PUBLISH_SCOPES" "1"; Ok "Posting permissions switched on" } else { Warn "Posting permissions are off. Reading works; run this command again later and answer y." }

  Write-Host "   Waiting for Render to restart with the new keys " -NoNewline
  Start-Sleep 10
  for ($i = 0; $i -lt 50; $i++) {
    Start-Sleep 10; Write-Host "." -NoNewline
    try { $d = Invoke-RestMethod -Uri "$base/deploys?limit=1" -Headers $H; if (@($d)[0].deploy.status -eq "live") { break } } catch {}
  }
  Write-Host ""
  Ok "Now click Connect on each card. Use your Facebook login for both Facebook and Instagram, and TikTok: techetx"
  Start-Process "$Site/dashboard?tab=socialc"
}

# ---------------------------------------------------------------- 5. Firebase email sign-in
Step "5/6" "Switching on email sign-in (Firebase)"
try {
  $cfgPath = Join-Path $env:USERPROFILE ".config\configstore\firebase-tools.json"
  $cfg = Get-Content $cfgPath -Raw | ConvertFrom-Json
  $form = @{
    client_id     = "563584335869-fgrhgmd47bqnekij5i8b5pr03ho849e6.apps.googleusercontent.com"
    client_secret = "j9iVZfS8kkCEFUPaAeJV0sAi"
    refresh_token = $cfg.tokens.refresh_token
    grant_type    = "refresh_token"
  }
  $tok = Invoke-RestMethod -Method Post -Uri "https://oauth2.googleapis.com/token" -Body $form
  $fh = @{ Authorization = "Bearer $($tok.access_token)"; "X-Goog-User-Project" = $FirebaseProject; "Content-Type" = "application/json" }
  $body = '{"signIn":{"email":{"enabled":true,"passwordRequired":true}}}'
  Invoke-RestMethod -Method Patch -Uri "https://identitytoolkit.googleapis.com/admin/v2/projects/$FirebaseProject/config?updateMask=signIn.email" -Headers $fh -Body $body | Out-Null
  Ok "Email/Password sign-in is on"
} catch {
  Warn "Could not switch it on automatically ($($_.Exception.Message))."
  Warn "Do this once: open https://console.firebase.google.com/project/$FirebaseProject/authentication/providers , click Email/Password, turn on the first switch, Save."
  Start-Process "https://console.firebase.google.com/project/$FirebaseProject/authentication/providers"
}

# ---------------------------------------------------------------- 6. build exe + apk
Step "6/6" "Building the Windows .exe and Android .apk on GitHub (about 10 minutes)"
if (-not $BuildApps) { Warn "App build skipped (it only runs when you add -BuildApps at the very end). The website and phone layout are already updated." } else {
try {
  Start-Sleep 10
  $busy = (gh run list --repo $Repo --workflow build-apps.yml --limit 3 --json status,databaseId --jq ".[] | select(.status==\"in_progress\" or .status==\"queued\") | .databaseId" | Out-String).Trim()
  if (-not $busy) {
    gh workflow run build-apps.yml --repo $Repo --ref main | Out-Null
    Start-Sleep 15
  }
  $id = (gh run list --repo $Repo --workflow build-apps.yml --limit 1 --json databaseId --jq ".[0].databaseId" | Out-String).Trim()
  if (-not $id) { throw "No build run found" }
  Write-Host "   Watching build $id ..."
  gh run watch $id --repo $Repo | Out-Null
  $result = (gh run view $id --repo $Repo --json conclusion --jq ".conclusion" | Out-String).Trim()
  if ($result -eq "success") {
    if (Test-Path $Out) { Remove-Item $Out -Recurse -Force }
    gh run download $id --repo $Repo --dir $Out | Out-Null
    $files = Get-ChildItem $Out -Recurse -Include *.exe, *.apk -ErrorAction SilentlyContinue
    foreach ($f in $files) { Ok $f.FullName; $Results += $f.FullName }
    explorer.exe $Out
  } else {
    Bad "The app build ended with: $result"
    $log = gh run view $id --repo $Repo --log-failed 2>&1 | Select-Object -Last 60
    $log | Out-File "$HOME\Downloads\blitz-actions-error.txt"
    $log | Select-Object -Last 25 | ForEach-Object { Write-Host $_ }
    Warn "Send Downloads\blitz-actions-error.txt to Claude and I'll fix it."
  }
} catch {
  Bad "Build step failed: $($_.Exception.Message)"
}
}

Write-Host ""
Write-Host "================  DONE  ================" -ForegroundColor Green
$Results | ForEach-Object { Write-Host "  $_" }
Write-Host "  Admin console: $Site/admin"
Write-Host "  Windows app: run the ficx-Setup-*.exe in $Out"
Write-Host "  Android app: copy app-debug.apk from $Out to your phone and open it"
if (Test-Path "$Out") { } else { Write-Host "  (If the apps are missing, see the messages above.)" }
Start-Process "$Site/app"
