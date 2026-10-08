# Media delivery: private R2 → TYCDN/CDNfly → XGPlayer

```
Authorized source
  → migration tool (optional, progressive)
  → private Cloudflare R2 bucket (dongman)
  → TYCDN / CDNfly origin auth (aws_s3)
  → browser XGPlayer (HLS)
```

JSON crawler/import files under `data/` are **not** public media. They stay on the app/migration host.

## Critical path rule

CDNfly private-R2 origin requires the **bucket name in the CDN URL path**:

- Working: `https://MEDIA_CDN_HOST/dongman/posters/100.jpg`
- Not working: `https://MEDIA_CDN_HOST/posters/100.jpg`

Application URL builders always insert `/{R2_BUCKET}/` before the object key.

## Environment

| Variable | Purpose |
| --- | --- |
| `R2_BUCKET` | Bucket name and CDN path prefix (production: `dongman`) |
| `R2_ENDPOINT` | Private S3 API endpoint (migration/upload only) |
| `R2_REGION` | R2 region (`auto`) |
| `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` | **Migration/upload** credentials only; never send to React |
| `MEDIA_CDN_BASE_URL` | Browser-facing CDN origin (preferred) |
| `CDN_BASE_URL` | Fallback if `MEDIA_CDN_BASE_URL` empty |
| `CDN_URL_AUTH_MODE` | `none` (default, CDNfly origin auth) or `legacy_hmac` (VPlayer `exp`/`sig`; **not** CDNfly-compatible) |
| `CDN_SIGN_SECRET` | Only for `legacy_hmac` |

Do **not** hardcode temporary test hostnames (`r2test…`, `cdn666…`) in code.

Prefer separate credentials:

- CDNfly origin: read-only to R2
- Migration tooling: write (least privilege)

## Object keys (stored in DB)

Prefer stable keys, not hostnames:

| Asset | Example key |
| --- | --- |
| Poster | `posters/olehdtv/<hash>.jpg` |
| Episode HLS | `videos/<videoId>/<episodeId>/index.m3u8` |
| Segments | `videos/<videoId>/<episodeId>/segment000.ts` (relative from playlist) |

HLS playlists should use **relative** segment URIs so they resolve under the same CDN path.

## Runtime URL resolution

1. If value is already `http(s)://…` → keep (legacy external source)
2. If `hls_object_key` / `cover_r2_key` is set → `MEDIA_CDN_BASE_URL/{bucket}/{key}`
3. Else fall back to `playback_url` / `poster_source_url`

## Migration CLI

From `backend/`:

```bash
# Plan + playlist inspect only (no writes)
go run ./cmd/migratemedia --dry-run --limit 1
go run ./cmd/migratemedia --dry-run --episode-id 53

# Real migration (explicit confirmation required)
go run ./cmd/migratemedia --execute --confirm=MIGRATE --episode-id 53
```

Rules:

- Omitting both flags refuses to run.
- `--execute` without `--confirm=MIGRATE` refuses to run.
- Dry-run may **read** the source playlist to classify media vs master; it never uploads or updates migration fields.
- Existing HLS media playlists are rewritten so segments are relative R2 objects (not the original provider).
- Master playlists are rejected until multi-variant support exists.
- On failure, `playback_url` stays intact so the episode remains playable from the source.

Unauthorized / membership / empty streams are skipped; metadata is preserved.

## TYCDN cache recommendations (configure in panel)

| Asset | Suggestion |
| --- | --- |
| Posters (`jpg/png/webp`) | Long TTL |
| HLS segments (`.ts` / `.m4s`) | Long TTL (immutable after publish) |
| VOD playlists (`.m3u8`) | Moderate/long TTL for finalized VOD |

## CORS

If the VPlayer app origin differs from `MEDIA_CDN_BASE_URL`, allow that app origin on the media CDN for GET (and Range) of HLS assets. Prefer explicit origins over `*` in production.

## DNS

Point a stable production hostname (e.g. `media.<domain>`) at TYCDN/CDNfly and set `MEDIA_CDN_BASE_URL` accordingly. No application rebuild required when the hostname changes.

## Security

- Keep R2 private; do not use r2.dev public delivery for production.
- Never put R2/CDN origin secrets in React, API JSON, git, or docs examples.
- `internal/play.SignURL` (`exp`/`sig`) is **not** proven compatible with CDNfly URL auth; leave `CDN_URL_AUTH_MODE=none` unless you add a CDNfly-compatible signer later.
