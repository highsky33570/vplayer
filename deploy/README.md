# VPlayer production deployment (Plan 1: GitHub → server)

This document describes the normal deploy workflow for the production host.
Application secrets live **outside** the Git checkout.

## Production paths

| Role | Path |
|------|------|
| Git checkout | `/opt/vplayer-repo` |
| Persistent env (secrets) | `/opt/vplayer/.env` |
| Repo env link | `/opt/vplayer-repo/.env` → `/opt/vplayer/.env` (symlink only) |
| API binary | `/usr/local/bin/vplayer-api` |
| API unit | `vplayer-api.service` |
| Worker binaries | `/opt/vplayer/backend/bin/syncincremental`, `enrichpending`, `migratemedia` |
| Leaf CLI binaries | `/opt/vplayer/backend/bin/syncleafmembership`, `syncleafimport` — **manual CLI only**; no systemd service; no timer; not executed by deploy |
| Backend SQL migrations (runtime) | `/opt/vplayer/backend/migrations` (from repo `backend/migrations`) |
| Discovery unit + timer | `vplayer-syncincremental.service` / `.timer` |
| Enrichment unit + timer | `vplayer-enrichpending.service` / `.timer` |
| Media migration unit + timer | `vplayer-migratemedia.service` / `.timer` (timer enabled by deploy; oneshot not started) |
| DB backup unit + timer | `vplayer-db-backup.service` / `.timer` (timer enabled by deploy; dump **not** started by deploy) |
| Healthcheck unit + timer | `vplayer-healthcheck.service` / `.timer` (~every 15m; journald + optional `/var/lib/vplayer/health.status`) |
| Ops scripts (runtime) | `/opt/vplayer/deploy/ops/` |
| MySQL DB backups | `/opt/vplayer/backups/mysql/` (mode 700; **not** deploy rollback) |
| Frontend dist (nginx) | `/opt/vplayer/frontend/dist` |
| Deploy rollback backups | `/opt/vplayer/releases/backups/<timestamp>-<shortsha>/` |

Deploy script: `deploy/deploy-production.sh` (run from the checkout as root).

## Local (developer) workflow

```bash
git status
# run relevant tests locally as needed
cd backend && go test ./...
cd ../frontend && npm test && npm run build

git add <files>
git commit -m "…"
git push origin main
```

Only commits that are on `origin/main` are deployable.

## Server workflow

```bash
cd /opt/vplayer-repo
sudo ./deploy/deploy-production.sh
```

What the script does (summary):

1. Requires root; refuses dirty **tracked** Git changes (ignored `.env` symlink is OK).
2. `git fetch origin main` and hard-updates the checkout to the exact `origin/main` commit (no merge).
3. Ensures `/opt/vplayer-repo/.env` is a symlink to `/opt/vplayer/.env` (never copies secrets into Git).
4. Aborts if `vplayer-syncincremental.service`, `vplayer-enrichpending.service`, or `vplayer-migratemedia.service` is currently active, and aborts if production worker/CLI binaries `syncincremental`, `enrichpending`, `migratemedia`, `syncleafmembership`, or `syncleafimport` are executing. An early check runs before build/staging; a final check runs again immediately before `INSTALL_STARTED` / first runtime binary install.
5. Builds/tests into a staging directory first (`go test`, API + worker Go binaries including `migratemedia`, `syncleafmembership`, `syncleafimport`, stages `backend/migrations`, `npm ci` + frontend build). Does **not** execute leaf CLIs.
6. After success only: backs up current API/worker/leaf CLI binaries (if present), frontend dist, and runtime migrations dir (not `.env`). First deploy of `syncleafimport` safely skips restore when no prior backup exists.
7. Atomically installs binaries (temp file + `mv`), swaps frontend dist via rename, then atomically swaps `/opt/vplayer/backend/migrations` (prepare `.new` fully → rename; dirs `755`, SQL files `644`).
8. Restarts **only** `vplayer-api.service` **after** migrations are in place so startup can discover/apply new SQL. Does **not** start oneshot workers. Does **not** run a separate migrate CLI. Does **not** run `syncleafmembership` or `syncleafimport`.
9. Installs/updates `vplayer-migratemedia.service` + `.timer`, `daemon-reload`, `enable` + `restart` on the **timer only** (never starts the migratemedia oneshot; timer must not `Requires=` the oneshot).
10. Installs Plan 8 ops scripts under `/opt/vplayer/deploy/ops/`, ensures `/opt/vplayer/backups/mysql` exists (mode 700), installs `vplayer-db-backup` + `vplayer-healthcheck` units, enables/restarts **those timers only**. Does **not** start a DB dump or restore during deploy.
11. Verifies API + sync/enrichment/migration/DB-backup/health **timers** active, migratemedia and db-backup **services** not active, `nginx -t`, local `/health`, local homepage HTTP, and local `GET /api/v1/videos?limit=1` (must be HTTP 200).
12. Keeps the newest 5 **deploy rollback** directories under `/opt/vplayer/releases/backups/` (separate from MySQL dumps).

MySQL backup/restore runbook: [`docs/BACKUP_RESTORE.md`](../docs/BACKUP_RESTORE.md).

Success line: `DEPLOYMENT SUCCEEDED`  
Failure after install began: automatic rollback attempt, then `DEPLOYMENT FAILED / ROLLED BACK`.

## Rollback

- Automatic: if install/restart/verify fails after production files were changed, the script restores the backup created for **this** run (binaries, frontend dist, and **migration files** when a migrations backup exists) and restarts `vplayer-api.service`.
- Migration rollback restores **files only** under `/opt/vplayer/backend/migrations`. It does **not** reverse database schema changes already applied by the API on startup.
- Manual (example):

```bash
BACKUP=/opt/vplayer/releases/backups/<timestamp>-<shortsha>
# restore binaries with atomic replace, restore dist, restore migrations/ if present, then:
sudo systemctl restart vplayer-api.service
```

Do not restore or overwrite `/opt/vplayer/.env` from Git.

## Safety notes

- Do not run the deploy script from a laptop against production paths unless you intend a real deploy.
- Timers stay enabled; oneshot workers are not interrupted mid-run (deploy aborts if a worker unit is active).
- `vplayer-api` WorkingDirectory is `/opt/vplayer/backend`, so it discovers SQL under `./migrations` at startup (existing API behavior). The deploy script copies repo migrations there before restart and does not invoke a separate migrate CLI.
- `deploy/systemd/vplayer-migratemedia.*` are installed by the deploy script (timer enabled; oneshot not started). Enrichment/discovery unit files remain host-managed examples unless already present on the server.
- Plan 8 DB dumps live under `/opt/vplayer/backups/mysql` and must not be web-served. Deploy rollback under `releases/backups` does **not** include MySQL data. Redis and R2 are not backed up in Plan 8.
