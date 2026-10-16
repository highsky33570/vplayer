# VPlayer backup & restore (Plan 8)

## Two different backup systems

| Kind | Location | Purpose |
|------|----------|---------|
| **Deploy rollback** | `/opt/vplayer/releases/backups/<timestamp>-<sha>/` | Binaries, frontend dist, migration **files** only. Created by `deploy-production.sh`. |
| **MySQL DB backup** | `/opt/vplayer/backups/mysql/` | Full logical dump of application data (users, catalog, episodes, sync/ops state). |

They are **not** interchangeable. Deploy rollback does **not** protect MySQL data.

## What Plan 8 does **not** back up

- **Redis** (sessions/cache; rebuildable)
- **R2 / media objects** (reproducible via preserved `playback_url` + `migratemedia`)
- **External alerting** (future work)

## Security

- DB dumps contain **sensitive** user and application data.
- Directory expectations: `/opt/vplayer/backups` and `…/mysql` mode **700**; dump files mode **600**.
- **Must not** be web-served (keep outside nginx `root` / aliases).
- Backup script does **not** source `/opt/vplayer/.env`.
- Credentials: uses the MySQL container environment (`MYSQL_USER` / `MYSQL_PASSWORD` / `MYSQL_DATABASE`) via `docker exec`, writing a short-lived mode-**600** `--defaults-extra-file` **inside** the container with MySQL option-file escaping (`deploy/ops/lib/mysql_defaults.sh`). The password is never passed as `mysql -p…` / `mysqldump -p…` and is not placed on the client argv. Values containing CR/LF are rejected (fail closed). Tradeoff: anyone who can `docker exec` as root into that container can already access MySQL.

## Automatic DB backup

| Item | Value |
|------|--------|
| Script | `/opt/vplayer/deploy/ops/vplayer-db-backup.sh` |
| Service | `vplayer-db-backup.service` (oneshot) |
| Timer | `vplayer-db-backup.timer` — daily ~`03:30` + up to 45m random delay |
| Filename | `vplayer-mysql-YYYYMMDDTHHMMSSZ.sql.gz` |
| Temp/incomplete | `*.sql.gz.partial` (validated with min size + `gzip -t` before rename; cleaned on failure/signals) |
| Retention | **14** completed dumps (recognized names only) |
| Lock | `/run/vplayer-db-backup.lock` (`flock`) |

### Manual backup

```bash
sudo systemctl start vplayer-db-backup.service
# or:
sudo /opt/vplayer/deploy/ops/vplayer-db-backup.sh
ls -lt /opt/vplayer/backups/mysql/
```

### Verify freshness

```bash
sudo /opt/vplayer/deploy/ops/vplayer-healthcheck.sh
# look for: PASS backup …  or WARN/FAIL backup …
```

Health treats **no backup yet** as **WARN**, and newest completed backup **>36h** as **FAIL**.

## Local health monitoring

| Item | Value |
|------|--------|
| Script | `/opt/vplayer/deploy/ops/vplayer-healthcheck.sh` |
| Timer | `vplayer-healthcheck.timer` — every 15 minutes (+ ≤2m random) |
| Status file | `/var/lib/vplayer/health.status` (`overall=…`, `checked_at=…`; dir `755`, file `644`, root-owned; no secrets) |
| Exit | `0` = OK or WARN; `1` = FAIL |

Checks: API `/health`, nginx homepage, MySQL container+ping, disk on `/opt/vplayer` (warn ≥80%, fail ≥90%), required systemd units/timers active, worker timer trigger freshness, DB backup freshness.

No email/webhook/Telegram in Plan 8.

## Restore (manual only — never automatic)

**NEVER** auto-restore. Always validate a dump on **staging** before any production import.

### Safe credential import (shared pattern)

Use the same helper as backup/healthcheck (`deploy/ops/lib/mysql_defaults.sh`). Do **not** put the DB password on the mysql/mysqldump argv, do **not** use bash process substitution into `sh`/`bash -c`, and do **not** paste passwords into shell history.

Pattern (host → `docker cp` helper → `docker exec -i … bash -s` with a mode-600 defaults file):

```bash
sudo docker cp /opt/vplayer/deploy/ops/lib/mysql_defaults.sh \
  "${TARGET_CONTAINER}:/tmp/mysql_defaults.sh"

gunzip -c /path/to/vplayer-mysql-YYYYMMDDTHHMMSSZ.sql.gz \
  | sudo docker exec -i "${TARGET_CONTAINER}" bash -s <<'EOS'
set -euo pipefail
source /tmp/mysql_defaults.sh
conf=$(mktemp)
trap 'rm -f "$conf" /tmp/mysql_defaults.sh' EXIT INT TERM HUP
write_mysql_client_defaults_file "$conf" "${MYSQL_USER}" "${MYSQL_PASSWORD}"
mysql --defaults-extra-file="$conf" "${MYSQL_DATABASE}"
EOS
```

### A. Staging validation (required before production)

Use a **non-production** MySQL container. Do **not** use the production container name here.

```bash
# EXPLICIT staging target — never set this to production accidentally
STAGING_MYSQL_CONTAINER='<STAGING_MYSQL_CONTAINER>'
DUMP=/opt/vplayer/backups/mysql/vplayer-mysql-YYYYMMDDTHHMMSSZ.sql.gz

echo "Staging container: ${STAGING_MYSQL_CONTAINER}"
# Expected: a dedicated staging/test container name, NOT production.

sudo docker cp /opt/vplayer/deploy/ops/lib/mysql_defaults.sh \
  "${STAGING_MYSQL_CONTAINER}:/tmp/mysql_defaults.sh"

gunzip -c "${DUMP}" | sudo docker exec -i "${STAGING_MYSQL_CONTAINER}" bash -s <<'EOS'
set -euo pipefail
source /tmp/mysql_defaults.sh
conf=$(mktemp)
trap 'rm -f "$conf" /tmp/mysql_defaults.sh' EXIT INT TERM HUP
write_mysql_client_defaults_file "$conf" "${MYSQL_USER}" "${MYSQL_PASSWORD}"
mysql --defaults-extra-file="$conf" "${MYSQL_DATABASE}"
EOS
```

After import, confirm `schema_migrations` and spot-check row counts **in staging** (same safe defaults-file approach as above, or your staging admin tool). Only then continue.

### B. Production restore (destructive — separate from staging)

Complete **all** of the following in order. Do not import while the API or workers can write.

1. Identify dump: `/opt/vplayer/backups/mysql/vplayer-mysql-….sql.gz` (already validated in staging).
2. Take a **fresh** pre-restore backup while production is still consistent enough to dump:
   `sudo systemctl start vplayer-db-backup.service`
3. Stop API and stop worker / backup timers (and ensure oneshots are not running):

```bash
sudo systemctl stop vplayer-api.service
sudo systemctl stop vplayer-syncincremental.timer \
  vplayer-enrichpending.timer \
  vplayer-migratemedia.timer \
  vplayer-db-backup.timer
sudo systemctl stop vplayer-syncincremental.service \
  vplayer-enrichpending.service \
  vplayer-migratemedia.service 2>/dev/null || true
```

4. Explicit confirmation (human): type the **production hostname** and dump basename.
5. Set and verify the production target **before** import:

```bash
# REQUIRED: operator must set this deliberately after confirmation
PROD_MYSQL_CONTAINER='vplayer-mysql'   # production only after step 4
echo "About to DESTROY/REPLACE data in: ${PROD_MYSQL_CONTAINER}"
# Re-read the line above. Abort unless it is the intended production container.
```

6. Destructive import (same safe defaults-file mechanism as staging):

```bash
DUMP=/opt/vplayer/backups/mysql/vplayer-mysql-YYYYMMDDTHHMMSSZ.sql.gz

sudo docker cp /opt/vplayer/deploy/ops/lib/mysql_defaults.sh \
  "${PROD_MYSQL_CONTAINER}:/tmp/mysql_defaults.sh"

gunzip -c "${DUMP}" | sudo docker exec -i "${PROD_MYSQL_CONTAINER}" bash -s <<'EOS'
set -euo pipefail
source /tmp/mysql_defaults.sh
conf=$(mktemp)
trap 'rm -f "$conf" /tmp/mysql_defaults.sh' EXIT INT TERM HUP
write_mysql_client_defaults_file "$conf" "${MYSQL_USER}" "${MYSQL_PASSWORD}"
mysql --defaults-extra-file="$conf" "${MYSQL_DATABASE}"
EOS
```

7. Verify `schema_migrations` (and spot-check users/videos counts) in production.
8. Start API; re-enable/start timers; run healthcheck:

```bash
sudo systemctl start vplayer-api.service
sudo systemctl start vplayer-syncincremental.timer \
  vplayer-enrichpending.timer \
  vplayer-migratemedia.timer \
  vplayer-db-backup.timer
sudo /opt/vplayer/deploy/ops/vplayer-healthcheck.sh
```

Plan 8 ships **documentation-only** restore (no restore timer, no auto-import helper).
