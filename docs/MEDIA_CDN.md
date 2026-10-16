# Media delivery: private R2 → browser-facing media host → XGPlayer

```
Authorized source
  → migration tool (optional, progressive)
  → private Cloudflare R2 bucket (dongman)
  → browser-facing media host (MEDIA_CDN_BASE_URL)
  → browser XGPlayer (HLS)
```

JSON crawler/import files under `data/` are **not** public media. They stay on the app/migration host.

## URL path modes (`MEDIA_CDN_PATH_MODE`)

Playback and cover URLs are built at request time from relative object keys (not stored as full CDN URLs).

| Mode | Result | When to use |
| --- | --- | --- |
| `bucket_prefix` (**default**) | `{MEDIA_CDN_BASE_URL}/{R2_BUCKET}/{key}` | Delivery hosts that expect the bucket name as the first path segment |
| `object_key` | `{MEDIA_CDN_BASE_URL}/{key}` | Delivery hosts / custom domains that already map to the R2 bucket root |

Examples (`base=https://media.example.com`, `bucket=dongman`, `key=videos/12/53/index.m3u8`):

- `bucket_prefix` → `https://media.example.com/dongman/videos/12/53/index.m3u8`
- `object_key` → `https://media.example.com/videos/12/53/index.m3u8`

Unset `MEDIA_CDN_PATH_MODE` keeps `bucket_prefix` (unchanged production behavior). Unsupported values fail at process startup.

HLS playlists use relative segment URIs, so segments resolve under the same delivery path in either mode.

## Environment

| Variable | Purpose |
| --- | --- |
| `R2_BUCKET` | Bucket name; also CDN path prefix when `MEDIA_CDN_PATH_MODE=bucket_prefix` |
| `R2_ENDPOINT` | Private S3 API endpoint (migration/upload only) |
| `R2_REGION` | R2 region (`auto`) |
| `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` | **Migration/upload** credentials only; never send to React |
| `MEDIA_CDN_BASE_URL` | Browser-facing media origin (preferred) |
| `CDN_BASE_URL` | Fallback if `MEDIA_CDN_BASE_URL` empty |
| `MEDIA_CDN_PATH_MODE` | `bucket_prefix` (default) or `object_key` |
| `CDN_URL_AUTH_MODE` | `none` (default) or `legacy_hmac` (VPlayer `exp`/`sig` on the delivery path) |
| `CDN_SIGN_SECRET` | Only for `legacy_hmac` |

Do **not** hardcode temporary test hostnames (`r2test…`, `cdn666…`) in code.

Prefer separate credentials:

- Delivery-origin / read path: least privilege read to R2 (when applicable)
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
2. If `hls_object_key` / `cover_r2_key` is set → build via `MEDIA_CDN_BASE_URL` + `MEDIA_CDN_PATH_MODE` (see above)
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

## Cache recommendations (on the media delivery host)

| Asset | Suggestion |
| --- | --- |
| Posters (`jpg/png/webp`) | Long TTL |
| HLS segments (`.ts` / `.m4s`) | Long TTL (immutable after publish) |
| VOD playlists (`.m3u8`) | Moderate/long TTL for finalized VOD |

## CORS

If the VPlayer app origin differs from `MEDIA_CDN_BASE_URL`, allow that app origin on the media host for GET (and Range) of HLS assets. Prefer explicit origins over `*` in production.

## DNS / hostname

Point a stable production hostname (e.g. `media.<domain>`) at your chosen media delivery front and set `MEDIA_CDN_BASE_URL` accordingly. Choose `MEDIA_CDN_PATH_MODE` to match how that host maps paths to R2 object keys. No application rebuild or media re-upload is required when only the hostname changes; switching path mode is a config change only.

## Security

- Keep the R2 S3 API private; do not use r2.dev public delivery for production.
- Never put R2/CDN origin secrets in React, API JSON, git, or docs examples.
- Leave `CDN_URL_AUTH_MODE=none` unless you intentionally enable VPlayer `exp`/`sig` signing against a compatible edge.
