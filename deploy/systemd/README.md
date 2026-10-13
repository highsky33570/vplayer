# VPlayer systemd examples (OLEHDTV incremental)

These unit files are **examples**. They are not installed or enabled by deploy automation.

## Services

| Unit | Role |
|------|------|
| `vplayer-syncincremental.service` + `.timer` | Bounded recent **discovery** (`syncincremental --limit 5 --trigger=timer`) — unit file not in this repo yet; live hosts must pass `--trigger=timer` when updated |
| `vplayer-enrichpending.service` + `.timer` | Bounded deferred **play enrichment** (`enrichpending --limit 15 --concurrency 1 --trigger=timer`) |

Keep `OLEHDTV_SYNC_ENABLED=false` on the API. Do not put secrets in unit files; use the same `EnvironmentFile` as `vplayer-api.service`.

## Suggested schedule

- Discovery: every 3 hours at `:00` (e.g. `00/3:00:00`)
- Enrichment: every 3 hours at `:20` (see `vplayer-enrichpending.timer`)

Enrichment only fetches already-stored `play_page_url` values (max 15–20 per run).

## Manual checks (do not enable until approved)

```bash
# Build
cd /opt/vplayer/backend
go build -o bin/enrichpending ./cmd/enrichpending

# Dry-run (HTTP allowed; zero DB writes)
./bin/enrichpending --dry-run --limit 5 --concurrency 1 --trigger=cli

# Timer-style invocation (matches systemd unit)
./bin/enrichpending --limit 15 --concurrency 1 --trigger=timer
# Discovery (when syncincremental unit is installed on the host):
# ./bin/syncincremental --limit 5 --trigger=timer

# Install examples (paths/User/EnvironmentFile must match the server)
# sudo cp deploy/systemd/vplayer-enrichpending.* /etc/systemd/system/
# sudo systemctl daemon-reload
# sudo systemctl start vplayer-enrichpending.service   # one manual run
# sudo systemctl enable --now vplayer-enrichpending.timer  # only when ready
```

Adjust `User=`, `WorkingDirectory=`, `EnvironmentFile=`, and binary paths to match `systemctl cat vplayer-api.service`.
