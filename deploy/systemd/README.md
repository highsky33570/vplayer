# VPlayer systemd units (catalog + media migration)

`vplayer-enrichpending.*` remain reference units (historically installed outside this script).
`vplayer-migratemedia.*` are installed by `deploy/deploy-production.sh` (timer enable + restart only).

## Services

| Unit | Role |
|------|------|
| `vplayer-syncincremental.service` + `.timer` | Bounded recent **discovery** (`syncincremental --limit 5 --trigger=timer`) — live host unit is the production pattern |
| `vplayer-enrichpending.service` + `.timer` | Bounded deferred **play enrichment** (`enrichpending --limit 15 --concurrency 1 --trigger=timer`) |
| `vplayer-migratemedia.service` + `.timer` | Selective R2 **media migration** (`migratemedia --execute --confirm=MIGRATE --limit 1 --trigger=timer`) |
| `vplayer-db-backup.service` + `.timer` | Daily MySQL logical backup (Plan 8; timer only from deploy) |
| `vplayer-healthcheck.service` + `.timer` | Local health monitoring every ~15m (Plan 8) |
| `vplayer-sourcecomments-incremental.service` + `.timer` | Incremental source-comment maintenance. Deploy installs the files and does **not** enable the timer |
| `vplayer-sourcecomments-full.service` + `.timer` | Weekly full source-comment repair. Deploy installs the files and does **not** enable the timer |

See [`docs/BACKUP_RESTORE.md`](../../docs/BACKUP_RESTORE.md) for DB backup location, retention, and restore. Deploy rollback under `/opt/vplayer/releases/backups` is separate.

Production migration worker pattern (match syncincremental):
- no `User=` / `Group=` (host has no `vplayer` Linux user)
- `WorkingDirectory=/opt/vplayer` so app dotenv loads `/opt/vplayer/.env`
- no `EnvironmentFile=` / no secrets in the unit
- timer must **not** `Requires=` the oneshot (that would start migration when the timer is activated)

## Suggested schedule

- Discovery: every 3 hours at `:00` (e.g. `00/3:00:00`)
- Enrichment: every 3 hours at `:20` (see `vplayer-enrichpending.timer`)
- Media migration: every 6 hours at `:40` + ~10min randomized delay; `Persistent=false` (`vplayer-migratemedia.timer`)
- DB backup: daily `03:30` + up to 45m randomized delay (`vplayer-db-backup.timer`)
- Healthcheck: every 15 minutes + up to 2m randomized delay (`vplayer-healthcheck.timer`)
- Source-comment incremental maintenance: every 3 hours at `:50` + up to 20m randomized delay (`vplayer-sourcecomments-incremental.timer`)
- Source-comment full repair: Sundays at `05:15` + up to 3h randomized delay (`vplayer-sourcecomments-full.timer`)

Source-comment timers are installed by deploy and left **disabled**. Deploy does not enable or start them. Both modes and historical backfill share MySQL advisory lock `vplayer_source_comment_sync`.

Enrichment only fetches already-stored `play_page_url` values (max 15–20 per run).
Migration processes at most one episode per timer fire (app advisory lock + cooldown).
Deploy does **not** start `vplayer-db-backup.service` (first dump is timer/manual).

## Manual checks

```bash
cd /opt/vplayer/backend
go build -o bin/migratemedia ./cmd/migratemedia

# Manual CLI (same dotenv as systemd WorkingDirectory=/opt/vplayer)
cd /opt/vplayer
./backend/bin/migratemedia --execute --confirm=MIGRATE --limit 1 --trigger=timer

# Deploy installs units then: systemctl enable TIMER; systemctl restart TIMER
# Never: systemctl start vplayer-migratemedia.service during deploy
```
