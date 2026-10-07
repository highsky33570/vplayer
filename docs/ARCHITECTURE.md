# Architecture

```
Browser (React + xgplayer)
    │  HTTPS API
    ▼
Go API (Gin) ── Redis (sessions, play tickets, hot lists)
    │
    ├── MySQL (users, videos, categories, play history)
    │
    ├── R2 (encrypted objects: source / HLS / covers)
    │
    └── TYCDN ←── origin pull / signed URLs ──→ R2
```

## Play flow

1. Client requests play URL for `video_id` (auth optional/required by config).
2. API issues short-lived ticket + signed m3u8 URL (CDN).
3. xgplayer loads HLS; segments prebuffer while playing.
4. HLS media may use AES-128 keys served only with valid ticket.

## Cover protection

Covers stored privately on R2; delivered via short-lived signed CDN/R2 URLs (not permanent public links).

## Admin

[gin-vue-admin](https://www.gin-vue-admin.com/) for CMS: categories, videos, upload/transcode jobs, ads, users. Calls same Go domain services or GVA-extended APIs.

## Modules (Go)

- `internal/user` — account
- `internal/catalog` — home/feed/category/search
- `internal/media` — R2, HLS job hooks, encryption keys
- `internal/play` — tickets, signed URLs
- `internal/adminapi` — hooks for GVA
