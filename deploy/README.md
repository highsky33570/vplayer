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
| Worker binaries | `/opt/vplayer/backend/bin/syncincremental`, `enrichpending` |
| Backend SQL migrations (runtime) | `/opt/vplayer/backend/migrations` (from repo `backend/migrations`) |
| Discovery unit + timer | `vplayer-syncincremental.service` / `.timer` |
| Enrichment unit + timer | `vplayer-enrichpending.service` / `.timer` |
| Frontend dist (nginx) | `/opt/vplayer/frontend/dist` |
| Deploy backups | `/opt/vplayer/releases/backups/<timestamp>-<shortsha>/` |

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
4. Aborts if `vplayer-syncincremental.service` or `vplayer-enrichpending.service` is currently active.
5. Builds/tests into a staging directory first (`go test`, three Go binaries, stages `backend/migrations`, `npm ci` + frontend build).
6. After success only: backs up current API/worker binaries, frontend dist, and runtime migrations dir (not `.env`).
7. Atomically installs binaries (temp file + `mv`), swaps frontend dist via rename, then atomically swaps `/opt/vplayer/backend/migrations` (prepare `.new` fully → rename; dirs `755`, SQL files `644`).
8. Restarts **only** `vplayer-api.service` **after** migrations are in place so startup can discover/apply new SQL. Does **not** start workers or change timers. Does **not** run a separate migrate CLI.
9. Verifies API + both timers active, `nginx -t`, local `/health`, and local homepage HTTP.
10. Keeps the newest 5 backup directories under `/opt/vplayer/releases/backups/`.

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
- Systemd unit files under `deploy/systemd/` are examples; this deploy script does not install or modify units.
