# VPlayer

Video VOD platform. Feature parity target: [olehdtv.com](https://www.olehdtv.com/). UI reference: [bilibili.com](https://www.bilibili.com/).

## Stack (final)

| Layer | Choice |
|-------|--------|
| User web | React + TypeScript |
| Public / media API | Go |
| Admin | [gin-vue-admin](https://www.gin-vue-admin.com/) |
| DB | MySQL |
| Cache | Redis |
| Object storage | Cloudflare R2 |
| CDN | Self-hosted TYCDN (VOD tweaks) |
| Protocol | HLS (`.m3u8` + segments), prebuffer while playing |
| Player | [西瓜播放器 xgplayer](https://github.com/bytedance/xgplayer) |
| Media security | Encrypt video (HLS AES) + protect covers (signed URLs) |

## Repo layout

```
VPlayer/
├── frontend/     # User site (React + TS + Vite)
├── backend/      # Go API (Gin): catalog, play tickets, users
├── admin/        # gin-vue-admin (see admin/README.md)
├── docs/         # Architecture & requirements
├── scripts/      # Dev helpers
└── docker-compose.yml
```

## Quick start (local)

```bash
# 1) Infra
docker compose up -d

# 2) Backend
cd backend && cp ../.env.example .env && go mod tidy && go run ./cmd/api

# 3) Frontend
cd frontend && npm install && npm run dev
```

- API: `http://localhost:8080`
- Web: `http://localhost:5173`
- MySQL: `localhost:3306` / Redis: `localhost:6379`

## Content ingest (important)

Admin supports **authorized upload / import** into R2, then transcode to HLS.

We do **not** ship a crawler that scrapes or re-hosts 欧乐/olehdtv (or any third-party) video catalog. That would violate copyright. Content must come from rights you own or license; playback may use partner-approved sources when contractually allowed.

## License

Private — client project.
