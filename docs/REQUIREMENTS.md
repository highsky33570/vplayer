# Final requirements (client) — locked

## Product

- Feature align: https://www.olehdtv.com/
- UI reference: https://www.bilibili.com/
- Responsive: desktop + mobile

## Stack

| Item | Choice |
|------|--------|
| Frontend | React + TypeScript |
| Backend | Go |
| Admin | https://www.gin-vue-admin.com/ |
| DB | MySQL |
| Cache | Redis |
| Storage | Cloudflare R2 |
| CDN | Self-built TYCDN (VOD tweaks) |
| Video | HLS `.m3u8` + segments, stream + prebuffer |
| Player | 西瓜播放器 https://github.com/bytedance/xgplayer |
| Security | Encrypt video (HLS AES path) + protect covers (signed URLs) |
| Ingest | Authorized sources / admin upload to R2 — **no pirate scrape of 欧乐** |

## Player

xgplayer + HLS: play/pause, seek, volume, speed, quality, fullscreen, captions hooks, ads hooks, buffer/reconnect, resume.
