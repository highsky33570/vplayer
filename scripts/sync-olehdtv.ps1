# VPlayer: offline MacCMS/SiteOne archive -> html_dump sync
#
# Does NOT live-crawl www.olehdtv.com.
# Put an authorized SiteOne offline export in -ArchiveDir, then run this script.
#
#   .\scripts\sync-olehdtv.ps1
#   .\scripts\sync-olehdtv.ps1 -ArchiveDir F:\VPlayer\data\olehdtv-html
#   .\scripts\sync-olehdtv.ps1 -SkipSync

param(
  [string]$ArchiveDir = "F:\VPlayer\data\olehdtv-html",
  [string]$BaseURL = "https://maccms.local",
  [string]$SiteOneExe = "F:\VPlayer\.tools\siteone-crawler\siteone-crawler\siteone-crawler.exe",
  [string]$FixtureFallback = "F:\VPlayer\backend\testdata\maccms_html",
  [switch]$SkipSync
)

$ErrorActionPreference = "Continue"
$Backend = "F:\VPlayer\backend"

function Write-Step([string]$msg) {
  Write-Host ""
  Write-Host ("==> {0}" -f $msg) -ForegroundColor Cyan
}

Write-Step "SiteOne binary"
if (Test-Path $SiteOneExe) {
  Write-Host ("FOUND: {0}" -f $SiteOneExe)
  & $SiteOneExe --version
} else {
  Write-Host ("MISSING: {0}" -f $SiteOneExe)
  Write-Host "Install from https://github.com/janreges/siteone-crawler/releases"
}

Write-Step "Resolve archive directory"
$useDir = $ArchiveDir
$htmlCount = 0
if (Test-Path $ArchiveDir) {
  $htmlCount = @(Get-ChildItem -Path $ArchiveDir -Recurse -Include *.html, *.htm -File -ErrorAction SilentlyContinue).Count
}
Write-Host ("ArchiveDir={0} htmlFiles={1}" -f $ArchiveDir, $htmlCount)

if ($htmlCount -lt 1) {
  Write-Host ""
  Write-Host ("BLOCKER: No offline archive at {0}" -f $ArchiveDir) -ForegroundColor Yellow
  Write-Host "Live crawl of www.olehdtv.com is not run by this script." -ForegroundColor Yellow
  Write-Host "Place a SiteOne offline export there, then re-run." -ForegroundColor Yellow
  Write-Host ("Fallback fixtures: {0}" -f $FixtureFallback) -ForegroundColor Yellow
  $useDir = $FixtureFallback
  $BaseURL = "https://maccms.local"
}

Write-Host ("Using: {0}" -f $useDir)
Write-Host ("BaseURL: {0}" -f $BaseURL)

$env:OLEHDTV_SOURCE_MODE = "html_dump"
$env:OLEHDTV_HTML_DUMP_ROOT = $useDir
$env:OLEHDTV_PUBLIC_BASE_URL = $BaseURL
$env:Path = "C:\Go\bin;" + [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")

Write-Step "Parser tests"
Set-Location $Backend
& go test ./internal/olehdtv/maccms/ -count=1 -run "SiteOne|EngineCrawl|Parse"
if ($LASTEXITCODE -ne 0) {
  throw "parser tests failed"
}

Write-Host ("SkipSync={0}" -f [bool]$SkipSync)
if ($SkipSync) {
  Write-Host "SkipSync set - done."
  exit 0
}

Write-Step "html_dump sync"
& go run ./cmd/sync
$syncExit = $LASTEXITCODE
Write-Host ("sync exit={0}" -f $syncExit)
if ($syncExit -ne 0) {
  throw "sync failed"
}

Write-Step "API verification"
try {
  $page = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/videos?limit=100"
  Write-Host ("API total={0} returned={1} has_more={2}" -f $page.total, $page.data.Count, $page.has_more)
  $cats = Invoke-RestMethod -Uri "http://localhost:8080/api/v1/categories"
  Write-Host ("API categories={0}" -f $cats.data.Count)
} catch {
  Write-Host "API not reachable on :8080 - start: cd backend; go run ./cmd/api" -ForegroundColor Yellow
}

Write-Step "Done"
Write-Host ("Archive used: {0}" -f $useDir)
