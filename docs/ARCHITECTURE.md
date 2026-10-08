# Architecture

```
Browser (React + xgplayer)
    │  HTTPS API
    ▼
Go API (Gin) ── Redis (sessions, play tickets, hot lists)
    │
    ├── MySQL (users, videos, episodes, categories)
    │
    └── MEDIA_CDN_BASE_URL (TYCDN/CDNfly)
              └── origin auth ──→ private R2 bucket (path must include /{bucket}/…)
```

See [MEDIA_CDN.md](./MEDIA_CDN.md) for object keys, `/dongman/` path rule, migration dry-run, cache/CORS/DNS.

## Play flow

1. Client requests play URL for `video_id` (+ optional `sid`/`nid`).
2. API resolves episode: prefer `hls_object_key` → CDN URL under `/{R2_BUCKET}/…`; else legacy absolute `playback_url`.
3. xgplayer (+ hls plugin) loads the m3u8 from the CDN host.
4. Optional VPlayer play ticket remains in the JSON response for future auth; CDN URL auth defaults to `CDN_URL_AUTH_MODE=none` (CDNfly↔R2 origin auth).

## Cover protection

Posters: prefer `cover_r2_key` → CDN URL; else `poster_source_url` until migrated. R2 stays private; browsers never receive R2 credentials.

## Admin

[gin-vue-admin](https://www.gin-vue-admin.com/) for CMS: categories, videos, upload/transcode jobs, ads, users. Calls same Go domain services or GVA-extended APIs.

## Modules (Go)

- `internal/user` — account
- `internal/catalog` — home/feed/category/search
- `internal/media` — R2, HLS job hooks, encryption keys
- `internal/play` — tickets, signed URLs
- `internal/adminapi` — hooks for GVA
