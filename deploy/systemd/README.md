# VPlayer systemd units (catalog + media migration)

`vplayer-enrichpending.*` remain reference units (historically installed outside this script).
`vplayer-migratemedia.*` are installed/enabled by `deploy/deploy-production.sh` (timer only).

## Services

| Unit | Role |
|------|------|
| `vplayer-syncincremental.service` + `.timer` | Bounded recent **discovery** (`syncincremental --limit 5 --trigger=timer`) — unit file not in this repo yet; live hosts must pass `--trigger=timer` when updated |
| `vplayer-enrichpending.service` + `.timer` | Bounded deferred **play enrichment** (`enrichpending --limit 15 --concurrency 1 --trigger=timer`) |
| `vplayer-migratemedia.service` + `.timer` | Selective R2 **media migration** (`migratemedia --execute --confirm=MIGRATE --limit 1 --trigger=timer`) |

Keep `OLEHDTV_SYNC_ENABLED=false` on the API. Do not put secrets in unit files; use the same `EnvironmentFile` as `vplayer-api.service`.

## Suggested schedule

- Discovery: every 3 hours at `:00` (e.g. `00/3:00:00`)
- Enrichment: every 3 hours at `:20` (see `vplayer-enrichpending.timer`)
- Media migration: every 6 hours at `:40` + ~10min randomized delay; `Persistent=false` so deploy/enable does not catch up missed runs (`vplayer-migratemedia.timer`)

Enrichment only fetches already-stored `play_page_url` values (max 15–20 per run).
Migration processes at most one episode per timer fire (app advisory lock + cooldown).

## Manual checks

```bash
# Build
cd /opt/vplayer/backend
go build -o bin/enrichpending ./cmd/enrichpending
go build -o bin/migratemedia ./cmd/migratemedia

# Enrichment dry-run (HTTP allowed; zero DB writes)
./bin/enrichpending --dry-run --limit 5 --concurrency 1 --trigger=cli

# Timer-style invocations
./bin/enrichpending --limit 15 --concurrency 1 --trigger=timer
# ./bin/syncincremental --limit 5 --trigger=timer
./bin/migratemedia --execute --confirm=MIGRATE --limit 1 --trigger=timer

# Migratemedia units are installed by deploy-production.sh (timer enable --now only).
# Manual install (if needed):
# sudo cp deploy/systemd/vplayer-migratemedia.* /etc/systemd/system/
# sudo systemctl daemon-reload
# sudo systemctl enable --now vplayer-migratemedia.timer
# Do NOT start vplayer-migratemedia.service from deploy — that would run a migration immediately.
```

Adjust `User=`, `WorkingDirectory=`, `EnvironmentFile=`, and binary paths to match `systemctl cat vplayer-api.service`.
