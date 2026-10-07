# Recommended SiteOne flags for an authorized offline export YOU run yourself.
# This repository does not invoke a live crawl of www.olehdtv.com.
#
# Example (run only against sources you are authorized to archive):
#
#   .\.tools\siteone-crawler\siteone-crawler\siteone-crawler.exe `
#     --url="https://example.authorized.tld/" `
#     --offline-export-dir="F:\VPlayer\data\olehdtv-html" `
#     --offline-export-preserve-url-structure `
#     --offline-export-preserve-urls `
#     --max-concurrent-requests=2 `
#     --timeout=20 `
#     --delay=1000
#
# Then:
#   .\scripts\sync-olehdtv.ps1 -ArchiveDir F:\VPlayer\data\olehdtv-html
