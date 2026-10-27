#!/usr/bin/env bash
# VPlayer production deploy: GitHub origin/main -> this host.
# Safe to review; intended to run as root on the production server only.
set -Eeuo pipefail

readonly REPO_DIR="/opt/vplayer-repo"
readonly PROD_ROOT="/opt/vplayer"
readonly PROD_ENV="${PROD_ROOT}/.env"
readonly API_BIN="/usr/local/bin/vplayer-api"
readonly WORKER_BIN_DIR="${PROD_ROOT}/backend/bin"
readonly SYNC_BIN="${WORKER_BIN_DIR}/syncincremental"
readonly ENRICH_BIN="${WORKER_BIN_DIR}/enrichpending"
readonly MIGRATE_BIN="${WORKER_BIN_DIR}/migratemedia"
readonly LEAF_BIN="${WORKER_BIN_DIR}/syncleafmembership"
readonly LEAF_IMPORT_BIN="${WORKER_BIN_DIR}/syncleafimport"
readonly MAINTAIN_BIN="${WORKER_BIN_DIR}/maintainsourcecomments"
readonly FRONTEND_DIST="${PROD_ROOT}/frontend/dist"
readonly MIGRATIONS_SRC="${REPO_DIR}/backend/migrations"
readonly MIGRATIONS_RUNTIME="${PROD_ROOT}/backend/migrations"
readonly BACKUP_ROOT="${PROD_ROOT}/releases/backups"
readonly MAX_BACKUPS=5

readonly API_UNIT="vplayer-api.service"
readonly SYNC_UNIT="vplayer-syncincremental.service"
readonly ENRICH_UNIT="vplayer-enrichpending.service"
readonly MIGRATE_UNIT="vplayer-migratemedia.service"
readonly SYNC_TIMER="vplayer-syncincremental.timer"
readonly ENRICH_TIMER="vplayer-enrichpending.timer"
readonly MIGRATE_TIMER="vplayer-migratemedia.timer"
readonly DB_BACKUP_UNIT="vplayer-db-backup.service"
readonly DB_BACKUP_TIMER="vplayer-db-backup.timer"
readonly HEALTH_UNIT="vplayer-healthcheck.service"
readonly HEALTH_TIMER="vplayer-healthcheck.timer"
readonly MAINTAIN_INCREMENTAL_UNIT="vplayer-sourcecomments-incremental.service"
readonly MAINTAIN_INCREMENTAL_TIMER="vplayer-sourcecomments-incremental.timer"
readonly MAINTAIN_FULL_UNIT="vplayer-sourcecomments-full.service"
readonly MAINTAIN_FULL_TIMER="vplayer-sourcecomments-full.timer"
readonly SYSTEMD_UNIT_SRC="${REPO_DIR}/deploy/systemd"
readonly SYSTEMD_UNIT_DST="/etc/systemd/system"
readonly OPS_SRC="${REPO_DIR}/deploy/ops"
readonly OPS_DST="${PROD_ROOT}/deploy/ops"

# shellcheck source=lib/worker_process_guard.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lib/worker_process_guard.sh"

STAGING=""
BACKUP_DIR=""
INSTALL_STARTED=0
ROLLED_BACK=0
PREV_COMMIT=""
TARGET_COMMIT=""
TARGET_SHORT=""

log() { printf '%s\n' "$*"; }
err() { printf 'ERROR: %s\n' "$*" >&2; }

die() {
  err "$*"
  exit 1
}

require_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    die "must run as root (use: sudo ./deploy/deploy-production.sh)"
  fi
}

cleanup() {
  local code=$?
  # Do not let set -e abort cleanup/rollback mid-way.
  set +e
  if [[ -n "${STAGING}" && -d "${STAGING}" ]]; then
    rm -rf "${STAGING}"
  fi
  if [[ "${code}" -ne 0 ]]; then
    if [[ "${INSTALL_STARTED}" -eq 1 && "${ROLLED_BACK}" -eq 0 ]]; then
      err "deployment failed after production artifacts changed; attempting rollback…"
      if rollback_from_backup; then
        ROLLED_BACK=1
        log "DEPLOYMENT FAILED / ROLLED BACK"
      else
        err "ROLLBACK FAILED — inspect ${BACKUP_DIR:-unknown} and systemd status manually"
        log "DEPLOYMENT FAILED / ROLLED BACK"
      fi
    else
      log "DEPLOYMENT FAILED"
    fi
  fi
  set -e
  return "${code}"
}

trap cleanup EXIT

is_active() {
  systemctl is-active --quiet "$1"
}

atomic_install_bin() {
  local src="$1"
  local dest="$2"
  local dest_dir
  dest_dir="$(dirname "${dest}")"
  mkdir -p "${dest_dir}"
  local tmp
  tmp="$(mktemp "${dest_dir}/.$(basename "${dest}").XXXXXX")"
  # Copy then rename: avoids "Text file busy" on a running executable.
  cp -f "${src}" "${tmp}"
  chmod 755 "${tmp}"
  mv -f "${tmp}" "${dest}"
}

backup_path_if_exists() {
  local src="$1"
  local dest_name="$2"
  if [[ -e "${src}" || -L "${src}" ]]; then
    mkdir -p "${BACKUP_DIR}"
    cp -a "${src}" "${BACKUP_DIR}/${dest_name}"
  fi
}

# Normalize migrations tree permissions (dirs 755, files 644). Never keep world-writable 666.
normalize_migrations_perms() {
  local root="$1"
  [[ -d "${root}" ]] || return 0
  find "${root}" -type d -exec chmod 755 {} +
  find "${root}" -type f -exec chmod 644 {} +
}

migrations_has_sql() {
  local root="$1"
  [[ -d "${root}" ]] || return 1
  local f
  f="$(find "${root}" -maxdepth 1 -type f -name '*.sql' -print -quit)"
  [[ -n "${f}" ]]
}

# Install/update syncincremental oneshot + timer from the repo.
# Never starts the oneshot service. Enables and (re)starts the TIMER only so the
# next OnCalendar event can fire later — not an immediate discovery crawl.
install_syncincremental_units() {
  local svc_src="${SYSTEMD_UNIT_SRC}/vplayer-syncincremental.service"
  local timer_src="${SYSTEMD_UNIT_SRC}/vplayer-syncincremental.timer"
  [[ -f "${svc_src}" ]] || die "missing unit: ${svc_src}"
  [[ -f "${timer_src}" ]] || die "missing unit: ${timer_src}"

  install -m 644 "${svc_src}" "${SYSTEMD_UNIT_DST}/vplayer-syncincremental.service"
  install -m 644 "${timer_src}" "${SYSTEMD_UNIT_DST}/vplayer-syncincremental.timer"
  systemctl daemon-reload

  systemctl enable "${SYNC_TIMER}"
  systemctl restart "${SYNC_TIMER}"

  if is_active "${SYNC_UNIT}"; then
    die "${SYNC_UNIT} became active during timer install; refusing unexpected discovery run"
  fi
}

# Install/update enrichpending oneshot + timer from the repo (same pattern as discovery).
install_enrichpending_units() {
  local svc_src="${SYSTEMD_UNIT_SRC}/vplayer-enrichpending.service"
  local timer_src="${SYSTEMD_UNIT_SRC}/vplayer-enrichpending.timer"
  [[ -f "${svc_src}" ]] || die "missing unit: ${svc_src}"
  [[ -f "${timer_src}" ]] || die "missing unit: ${timer_src}"

  install -m 644 "${svc_src}" "${SYSTEMD_UNIT_DST}/vplayer-enrichpending.service"
  install -m 644 "${timer_src}" "${SYSTEMD_UNIT_DST}/vplayer-enrichpending.timer"
  systemctl daemon-reload

  systemctl enable "${ENRICH_TIMER}"
  systemctl restart "${ENRICH_TIMER}"

  if is_active "${ENRICH_UNIT}"; then
    die "${ENRICH_UNIT} became active during timer install; refusing unexpected enrichment run"
  fi
}

# Install/update migratemedia oneshot + timer from the repo.
# Never starts the oneshot service. Enables and (re)starts the TIMER only so the
# next OnCalendar event can fire later — not an immediate migration.
install_migratemedia_units() {
  local svc_src="${SYSTEMD_UNIT_SRC}/vplayer-migratemedia.service"
  local timer_src="${SYSTEMD_UNIT_SRC}/vplayer-migratemedia.timer"
  [[ -f "${svc_src}" ]] || die "missing unit: ${svc_src}"
  [[ -f "${timer_src}" ]] || die "missing unit: ${timer_src}"

  install -m 644 "${svc_src}" "${SYSTEMD_UNIT_DST}/vplayer-migratemedia.service"
  install -m 644 "${timer_src}" "${SYSTEMD_UNIT_DST}/vplayer-migratemedia.timer"
  systemctl daemon-reload

  # enable without --now, then start/restart the TIMER unit only.
  # Do not: systemctl enable --now (historical immediate oneshot pull via Requires=),
  # and never: systemctl start/restart the migratemedia.service oneshot.
  systemctl enable "${MIGRATE_TIMER}"
  systemctl restart "${MIGRATE_TIMER}"

  if is_active "${MIGRATE_UNIT}"; then
    die "${MIGRATE_UNIT} became active during timer install; refusing unexpected migration"
  fi
}

# Install source-comment maintenance unit files and reload systemd.
# Timers stay disabled. This function must never enable, start, or restart them.
install_source_comment_maintenance_units() {
  local inc_svc="${SYSTEMD_UNIT_SRC}/vplayer-sourcecomments-incremental.service"
  local inc_timer="${SYSTEMD_UNIT_SRC}/vplayer-sourcecomments-incremental.timer"
  local full_svc="${SYSTEMD_UNIT_SRC}/vplayer-sourcecomments-full.service"
  local full_timer="${SYSTEMD_UNIT_SRC}/vplayer-sourcecomments-full.timer"
  [[ -f "${inc_svc}" && -f "${inc_timer}" ]] || die "missing incremental maintenance units"
  [[ -f "${full_svc}" && -f "${full_timer}" ]] || die "missing full maintenance units"

  install -m 644 "${inc_svc}" "${SYSTEMD_UNIT_DST}/vplayer-sourcecomments-incremental.service"
  install -m 644 "${inc_timer}" "${SYSTEMD_UNIT_DST}/vplayer-sourcecomments-incremental.timer"
  install -m 644 "${full_svc}" "${SYSTEMD_UNIT_DST}/vplayer-sourcecomments-full.service"
  install -m 644 "${full_timer}" "${SYSTEMD_UNIT_DST}/vplayer-sourcecomments-full.timer"
  systemctl daemon-reload

  if is_active "${MAINTAIN_INCREMENTAL_UNIT}" || is_active "${MAINTAIN_FULL_UNIT}"; then
    die "source-comment maintenance became active during unit install"
  fi
}

# Install Plan 8 ops scripts + DB backup / healthcheck timers.
# Enables and restarts TIMERS only. Never starts a DB dump or restore during deploy.
install_plan8_ops_units() {
  local backup_svc="${SYSTEMD_UNIT_SRC}/vplayer-db-backup.service"
  local backup_timer="${SYSTEMD_UNIT_SRC}/vplayer-db-backup.timer"
  local health_svc="${SYSTEMD_UNIT_SRC}/vplayer-healthcheck.service"
  local health_timer="${SYSTEMD_UNIT_SRC}/vplayer-healthcheck.timer"

  [[ -f "${backup_svc}" && -f "${backup_timer}" ]] || die "missing db-backup systemd units"
  [[ -f "${health_svc}" && -f "${health_timer}" ]] || die "missing healthcheck systemd units"
  [[ -f "${OPS_SRC}/vplayer-db-backup.sh" ]] || die "missing ${OPS_SRC}/vplayer-db-backup.sh"
  [[ -f "${OPS_SRC}/vplayer-healthcheck.sh" ]] || die "missing ${OPS_SRC}/vplayer-healthcheck.sh"
  [[ -d "${OPS_SRC}/lib" ]] || die "missing ${OPS_SRC}/lib"

  [[ -f "${OPS_SRC}/lib/mysql_defaults.sh" ]] || die "missing ${OPS_SRC}/lib/mysql_defaults.sh"

  mkdir -p "${OPS_DST}/lib"
  install -m 755 "${OPS_SRC}/vplayer-db-backup.sh" "${OPS_DST}/vplayer-db-backup.sh"
  install -m 755 "${OPS_SRC}/vplayer-healthcheck.sh" "${OPS_DST}/vplayer-healthcheck.sh"
  install -m 644 "${OPS_SRC}/lib/backup_helpers.sh" "${OPS_DST}/lib/backup_helpers.sh"
  install -m 644 "${OPS_SRC}/lib/health_helpers.sh" "${OPS_DST}/lib/health_helpers.sh"
  install -m 644 "${OPS_SRC}/lib/mysql_defaults.sh" "${OPS_DST}/lib/mysql_defaults.sh"

  # Ensure backup root exists with tight perms (do not dump during deploy).
  mkdir -p "${PROD_ROOT}/backups/mysql"
  chmod 700 "${PROD_ROOT}/backups" "${PROD_ROOT}/backups/mysql"

  install -m 644 "${backup_svc}" "${SYSTEMD_UNIT_DST}/vplayer-db-backup.service"
  install -m 644 "${backup_timer}" "${SYSTEMD_UNIT_DST}/vplayer-db-backup.timer"
  install -m 644 "${health_svc}" "${SYSTEMD_UNIT_DST}/vplayer-healthcheck.service"
  install -m 644 "${health_timer}" "${SYSTEMD_UNIT_DST}/vplayer-healthcheck.timer"
  systemctl daemon-reload

  systemctl enable "${DB_BACKUP_TIMER}"
  systemctl restart "${DB_BACKUP_TIMER}"
  systemctl enable "${HEALTH_TIMER}"
  systemctl restart "${HEALTH_TIMER}"

  if is_active "${DB_BACKUP_UNIT}"; then
    die "${DB_BACKUP_UNIT} became active during timer install; refusing unexpected DB dump"
  fi
}

# Stage a complete migrations tree under STAGING (does not touch runtime).
stage_migrations() {
  [[ -d "${MIGRATIONS_SRC}" ]] || die "missing migrations source: ${MIGRATIONS_SRC}"
  local dest="${STAGING}/migrations"
  rm -rf "${dest}"
  mkdir -p "${dest}"
  cp -a "${MIGRATIONS_SRC}/." "${dest}/"
  normalize_migrations_perms "${dest}"
  migrations_has_sql "${dest}" || die "staged migrations has no .sql files (${dest})"
}

# Atomically replace runtime migrations: prepare .new fully, then rename into place.
# Never leaves a half-copied destination directory as the live path.
atomic_install_migrations() {
  local src="$1"
  local dest="$2"
  local dest_new="${dest}.new"
  local dest_old="${dest}.old"

  migrations_has_sql "${src}" || die "migrations source incomplete: ${src}"

  rm -rf "${dest_new}" "${dest_old}"
  mkdir -p "$(dirname "${dest}")"
  mkdir -p "${dest_new}"
  cp -a "${src}/." "${dest_new}/"
  normalize_migrations_perms "${dest_new}"
  migrations_has_sql "${dest_new}" || die "new migrations incomplete: ${dest_new}"

  if [[ -e "${dest}" || -L "${dest}" ]]; then
    mv -f "${dest}" "${dest_old}"
  fi
  mv -f "${dest_new}" "${dest}"
  rm -rf "${dest_old}"
}

rollback_from_backup() {
  if [[ -z "${BACKUP_DIR}" || ! -d "${BACKUP_DIR}" ]]; then
    err "no backup directory for this deployment"
    return 1
  fi

  local ok=1
  if [[ -f "${BACKUP_DIR}/vplayer-api" ]]; then
    atomic_install_bin "${BACKUP_DIR}/vplayer-api" "${API_BIN}" || ok=0
  else
    err "backup missing vplayer-api; skipping API binary restore"
  fi
  if [[ -f "${BACKUP_DIR}/syncincremental" ]]; then
    atomic_install_bin "${BACKUP_DIR}/syncincremental" "${SYNC_BIN}" || ok=0
  fi
  if [[ -f "${BACKUP_DIR}/enrichpending" ]]; then
    atomic_install_bin "${BACKUP_DIR}/enrichpending" "${ENRICH_BIN}" || ok=0
  fi
  if [[ -f "${BACKUP_DIR}/migratemedia" ]]; then
    atomic_install_bin "${BACKUP_DIR}/migratemedia" "${MIGRATE_BIN}" || ok=0
  fi
  if [[ -f "${BACKUP_DIR}/syncleafmembership" ]]; then
    atomic_install_bin "${BACKUP_DIR}/syncleafmembership" "${LEAF_BIN}" || ok=0
  fi
  if [[ -f "${BACKUP_DIR}/syncleafimport" ]]; then
    atomic_install_bin "${BACKUP_DIR}/syncleafimport" "${LEAF_IMPORT_BIN}" || ok=0
  fi
  if [[ -f "${BACKUP_DIR}/maintainsourcecomments" ]]; then
    atomic_install_bin "${BACKUP_DIR}/maintainsourcecomments" "${MAINTAIN_BIN}" || ok=0
  fi
  if [[ -d "${BACKUP_DIR}/frontend-dist" ]]; then
    local new_dist="${FRONTEND_DIST}.rollback-new"
    local old_dist="${FRONTEND_DIST}.rollback-old"
    rm -rf "${new_dist}" "${old_dist}"
    cp -a "${BACKUP_DIR}/frontend-dist" "${new_dist}" || ok=0
    if [[ -e "${FRONTEND_DIST}" || -L "${FRONTEND_DIST}" ]]; then
      mv -f "${FRONTEND_DIST}" "${old_dist}" || ok=0
    fi
    mv -f "${new_dist}" "${FRONTEND_DIST}" || ok=0
    rm -rf "${old_dist}"
  fi

  # Restore migration FILES only (never reverse applied DB schema).
  if [[ -d "${BACKUP_DIR}/migrations" ]]; then
    local new_mig="${MIGRATIONS_RUNTIME}.rollback-new"
    local old_mig="${MIGRATIONS_RUNTIME}.rollback-old"
    rm -rf "${new_mig}" "${old_mig}"
    mkdir -p "${new_mig}"
    if cp -a "${BACKUP_DIR}/migrations/." "${new_mig}/"; then
      normalize_migrations_perms "${new_mig}"
      if [[ -e "${MIGRATIONS_RUNTIME}" || -L "${MIGRATIONS_RUNTIME}" ]]; then
        mv -f "${MIGRATIONS_RUNTIME}" "${old_mig}" || ok=0
      fi
      if mv -f "${new_mig}" "${MIGRATIONS_RUNTIME}"; then
        rm -rf "${old_mig}"
        log "rollback: restored migrations files from backup"
      else
        err "rollback: failed to restore migrations directory"
        # Best effort: put previous live tree back if we moved it aside.
        if [[ -d "${old_mig}" && ! -e "${MIGRATIONS_RUNTIME}" ]]; then
          mv -f "${old_mig}" "${MIGRATIONS_RUNTIME}" || true
        fi
        rm -rf "${new_mig}"
        ok=0
      fi
    else
      err "rollback: failed to copy backed-up migrations"
      rm -rf "${new_mig}"
      ok=0
    fi
  else
    log "rollback: no migrations backup present (runtime migrations left as-is)"
  fi

  if systemctl restart "${API_UNIT}"; then
    sleep 1
    if is_active "${API_UNIT}"; then
      log "rollback: ${API_UNIT} is active"
    else
      err "rollback: ${API_UNIT} failed to become active"
      ok=0
    fi
  else
    err "rollback: failed to restart ${API_UNIT}"
    ok=0
  fi

  [[ "${ok}" -eq 1 ]]
}

prune_old_backups() {
  # Strictly limited to BACKUP_ROOT; never delete elsewhere.
  [[ -d "${BACKUP_ROOT}" ]] || return 0
  local -a entries=()
  local e
  while IFS= read -r e; do
    [[ -n "${e}" ]] && entries+=("${e}")
  done < <(find "${BACKUP_ROOT}" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort)

  local count="${#entries[@]}"
  local remove=$((count - MAX_BACKUPS))
  if (( remove <= 0 )); then
    return 0
  fi
  local i
  for ((i = 0; i < remove; i++)); do
    local name="${entries[$i]}"
    # Only delete names that look like our backup dirs (timestamp-shortsha).
    if [[ "${name}" =~ ^[0-9]{8}T[0-9]{6}Z-[0-9a-f]{7,40}$ ]]; then
      log "pruning old backup: ${BACKUP_ROOT}/${name}"
      rm -rf "${BACKUP_ROOT}/${name}"
    else
      err "skip prune of unexpected entry: ${name}"
    fi
  done
}

resolve_health_url() {
  # Read only HTTP_ADDR from the persistent env file; never dump the file.
  local addr=":8080"
  if [[ -f "${PROD_ENV}" ]]; then
    local line
    line="$(grep -E '^[[:space:]]*HTTP_ADDR=' "${PROD_ENV}" | tail -n1 || true)"
    if [[ -n "${line}" ]]; then
      addr="${line#*=}"
      addr="${addr%%$'\r'}"
      addr="${addr#\"}"
      addr="${addr%\"}"
      addr="${addr#\'}"
      addr="${addr%\'}"
    fi
  fi
  if [[ "${addr}" == :* ]]; then
    printf 'http://127.0.0.1%s/health' "${addr}"
  elif [[ "${addr}" =~ ^[0-9]+$ ]]; then
    printf 'http://127.0.0.1:%s/health' "${addr}"
  elif [[ "${addr}" =~ ^https?:// ]]; then
    printf '%s/health' "${addr%/}"
  else
    printf 'http://%s/health' "${addr}"
  fi
}

main() {
  require_root

  [[ -d "${REPO_DIR}/.git" ]] || die "missing git checkout: ${REPO_DIR}"
  cd "${REPO_DIR}"

  # Tracked modifications only — ignored .env symlink must not fail the deploy.
  if ! git diff --quiet HEAD -- || ! git diff --cached --quiet; then
    err "git working tree has tracked modifications; refuse to deploy"
    git status --short --untracked-files=no >&2 || true
    die "commit or discard tracked changes in ${REPO_DIR} first"
  fi

  log "fetching origin/main…"
  git fetch --prune origin main

  PREV_COMMIT="$(git rev-parse HEAD)"
  TARGET_COMMIT="$(git rev-parse origin/main)"
  TARGET_SHORT="$(git rev-parse --short "${TARGET_COMMIT}")"

  log "previous commit: ${PREV_COMMIT}"
  log "target commit:   ${TARGET_COMMIT}"

  if [[ "${PREV_COMMIT}" == "${TARGET_COMMIT}" ]]; then
    log "checkout already at origin/main (${TARGET_SHORT}); continuing with rebuild/reinstall"
  fi

  # Exact GitHub main tip — no merge. Do not git clean untracked files.
  git checkout -B main "${TARGET_COMMIT}"
  git reset --hard "${TARGET_COMMIT}"

  if [[ "$(git rev-parse HEAD)" != "${TARGET_COMMIT}" ]]; then
    die "checkout did not land on target ${TARGET_COMMIT}"
  fi

  [[ -f "${PROD_ENV}" ]] || die "missing persistent env file: ${PROD_ENV} (never invent secrets)"

  # Point repo .env at persistent production config (symlink only; never copy secrets into Git).
  if [[ -L "${REPO_DIR}/.env" ]]; then
    local current
    current="$(readlink -f "${REPO_DIR}/.env" || true)"
    local expected
    expected="$(readlink -f "${PROD_ENV}")"
    if [[ "${current}" != "${expected}" ]]; then
      ln -sfn "${PROD_ENV}" "${REPO_DIR}/.env"
      log "updated ${REPO_DIR}/.env symlink -> ${PROD_ENV}"
    fi
  elif [[ -e "${REPO_DIR}/.env" ]]; then
    die "${REPO_DIR}/.env exists and is not a symlink; refuse to overwrite (expected symlink to ${PROD_ENV})"
  else
    ln -s "${PROD_ENV}" "${REPO_DIR}/.env"
    log "created ${REPO_DIR}/.env -> ${PROD_ENV}"
  fi

  if is_active "${SYNC_UNIT}"; then
    die "${SYNC_UNIT} is active; aborting to avoid interrupting a running worker"
  fi
  if is_active "${ENRICH_UNIT}"; then
    die "${ENRICH_UNIT} is active; aborting to avoid interrupting a running worker"
  fi
  if is_active "${MIGRATE_UNIT}"; then
    die "${MIGRATE_UNIT} is active; aborting to avoid interrupting a running worker"
  fi
  # Admin-launched / timer oneshots may not appear as active units in every edge case.
  if is_active "${MAINTAIN_INCREMENTAL_UNIT}"; then
    die "${MAINTAIN_INCREMENTAL_UNIT} is active; aborting to avoid interrupting maintenance"
  fi
  if is_active "${MAINTAIN_FULL_UNIT}"; then
    die "${MAINTAIN_FULL_UNIT} is active; aborting to avoid interrupting maintenance"
  fi
  if worker_bins_running "${SYNC_BIN}" "${ENRICH_BIN}" "${MIGRATE_BIN}" "${LEAF_BIN}" "${LEAF_IMPORT_BIN}" "${MAINTAIN_BIN}"; then
    die "deployment refused: VPlayer worker process is active"
  fi

  local stamp
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  STAGING="$(mktemp -d /tmp/vplayer-deploy.XXXXXX)"
  BACKUP_DIR="${BACKUP_ROOT}/${stamp}-${TARGET_SHORT}"

  log "staging builds in ${STAGING}"
  mkdir -p "${STAGING}/bin" "${STAGING}/frontend-dist"

  # --- Build & test (staging only; production artifacts untouched) ---
  log "backend: go test ./..."
  (
    cd "${REPO_DIR}/backend"
    go test ./...
  )

  log "backend: build cmd/api, syncincremental, enrichpending, migratemedia, syncleafmembership, syncleafimport, maintainsourcecomments"
  (
    cd "${REPO_DIR}/backend"
    go build -o "${STAGING}/bin/vplayer-api" ./cmd/api
    go build -o "${STAGING}/bin/syncincremental" ./cmd/syncincremental
    go build -o "${STAGING}/bin/enrichpending" ./cmd/enrichpending
    go build -o "${STAGING}/bin/migratemedia" ./cmd/migratemedia
    go build -o "${STAGING}/bin/syncleafmembership" ./cmd/syncleafmembership
    go build -o "${STAGING}/bin/syncleafimport" ./cmd/syncleafimport
    go build -o "${STAGING}/bin/maintainsourcecomments" ./cmd/maintainsourcecomments
  )

  log "staging backend SQL migrations…"
  stage_migrations

  log "frontend: npm ci && npm run build"
  (
    cd "${REPO_DIR}/frontend"
    npm ci
    npm run build
  )
  [[ -d "${REPO_DIR}/frontend/dist" ]] || die "frontend build did not produce dist/"
  # Copy completed dist into staging (leave repo dist alone).
  cp -a "${REPO_DIR}/frontend/dist/." "${STAGING}/frontend-dist/"
  [[ -f "${STAGING}/frontend-dist/index.html" ]] || die "staged frontend dist missing index.html"

  # --- Backups (only after all builds succeed; does not mutate live runtime) ---
  log "creating rollback backup under ${BACKUP_DIR}"
  mkdir -p "${BACKUP_DIR}"
  backup_path_if_exists "${API_BIN}" "vplayer-api"
  backup_path_if_exists "${SYNC_BIN}" "syncincremental"
  backup_path_if_exists "${ENRICH_BIN}" "enrichpending"
  backup_path_if_exists "${MIGRATE_BIN}" "migratemedia"
  backup_path_if_exists "${LEAF_BIN}" "syncleafmembership"
  backup_path_if_exists "${LEAF_IMPORT_BIN}" "syncleafimport"
  backup_path_if_exists "${MAINTAIN_BIN}" "maintainsourcecomments"
  if [[ -d "${FRONTEND_DIST}" ]]; then
    cp -a "${FRONTEND_DIST}" "${BACKUP_DIR}/frontend-dist"
  fi
  if [[ -d "${MIGRATIONS_RUNTIME}" ]]; then
    cp -a "${MIGRATIONS_RUNTIME}" "${BACKUP_DIR}/migrations"
  fi

  # FINAL worker/systemd guard — immediately before first runtime mutation.
  if is_active "${SYNC_UNIT}"; then
    die "${SYNC_UNIT} is active; aborting to avoid interrupting a running worker"
  fi
  if is_active "${ENRICH_UNIT}"; then
    die "${ENRICH_UNIT} is active; aborting to avoid interrupting a running worker"
  fi
  if is_active "${MIGRATE_UNIT}"; then
    die "${MIGRATE_UNIT} is active; aborting to avoid interrupting a running worker"
  fi
  if is_active "${MAINTAIN_INCREMENTAL_UNIT}"; then
    die "${MAINTAIN_INCREMENTAL_UNIT} is active; aborting to avoid interrupting maintenance"
  fi
  if is_active "${MAINTAIN_FULL_UNIT}"; then
    die "${MAINTAIN_FULL_UNIT} is active; aborting to avoid interrupting maintenance"
  fi
  if worker_bins_running "${SYNC_BIN}" "${ENRICH_BIN}" "${MIGRATE_BIN}" "${LEAF_BIN}" "${LEAF_IMPORT_BIN}" "${MAINTAIN_BIN}"; then
    die "deployment refused: VPlayer worker process is active"
  fi

  # --- Install (production mutation begins) ---
  INSTALL_STARTED=1
  log "installing binaries (atomic rename)…"
  mkdir -p "${WORKER_BIN_DIR}" "$(dirname "${API_BIN}")" "$(dirname "${FRONTEND_DIST}")"

  atomic_install_bin "${STAGING}/bin/vplayer-api" "${API_BIN}"
  atomic_install_bin "${STAGING}/bin/syncincremental" "${SYNC_BIN}"
  atomic_install_bin "${STAGING}/bin/enrichpending" "${ENRICH_BIN}"
  atomic_install_bin "${STAGING}/bin/migratemedia" "${MIGRATE_BIN}"
  atomic_install_bin "${STAGING}/bin/syncleafmembership" "${LEAF_BIN}"
  atomic_install_bin "${STAGING}/bin/syncleafimport" "${LEAF_IMPORT_BIN}"
  atomic_install_bin "${STAGING}/bin/maintainsourcecomments" "${MAINTAIN_BIN}"

  log "replacing frontend dist…"
  local dist_new="${FRONTEND_DIST}.new"
  local dist_old="${FRONTEND_DIST}.old"
  rm -rf "${dist_new}" "${dist_old}"
  mkdir -p "${dist_new}"
  cp -a "${STAGING}/frontend-dist/." "${dist_new}/"
  [[ -f "${dist_new}/index.html" ]] || die "new frontend dist incomplete"
  if [[ -e "${FRONTEND_DIST}" || -L "${FRONTEND_DIST}" ]]; then
    mv -f "${FRONTEND_DIST}" "${dist_old}"
  fi
  mv -f "${dist_new}" "${FRONTEND_DIST}"
  rm -rf "${dist_old}"

  # Migrations must be live before API restart so startup ApplyMigrationsDir can see new SQL.
  log "deploying backend SQL migrations to ${MIGRATIONS_RUNTIME}…"
  atomic_install_migrations "${STAGING}/migrations" "${MIGRATIONS_RUNTIME}"

  log "restarting ${API_UNIT} only (oneshot workers not started)…"
  systemctl restart "${API_UNIT}"
  sleep 1

  log "installing ${SYNC_UNIT} / ${SYNC_TIMER} (enable+restart timer only; never start oneshot)…"
  install_syncincremental_units

  log "installing ${ENRICH_UNIT} / ${ENRICH_TIMER} (enable+restart timer only; never start oneshot)…"
  install_enrichpending_units

  log "installing ${MIGRATE_UNIT} / ${MIGRATE_TIMER} (enable+restart timer only; never start oneshot)…"
  install_migratemedia_units

  log "installing source-comment maintenance units (files and daemon-reload only; timers stay disabled)…"
  install_source_comment_maintenance_units

  log "installing Plan 8 ops (DB backup + healthcheck timers; no dump during deploy)…"
  install_plan8_ops_units

  # --- Verify ---
  log "verifying units and endpoints…"
  is_active "${API_UNIT}" || die "${API_UNIT} is not active after restart"
  is_active "${SYNC_TIMER}" || die "${SYNC_TIMER} is not active"
  is_active "${ENRICH_TIMER}" || die "${ENRICH_TIMER} is not active"
  is_active "${MIGRATE_TIMER}" || die "${MIGRATE_TIMER} is not active"
  is_active "${DB_BACKUP_TIMER}" || die "${DB_BACKUP_TIMER} is not active"
  is_active "${HEALTH_TIMER}" || die "${HEALTH_TIMER} is not active"
  if is_active "${MIGRATE_UNIT}"; then
    die "${MIGRATE_UNIT} is unexpectedly active after deploy (migration must not run from deploy)"
  fi
  if is_active "${DB_BACKUP_UNIT}"; then
    die "${DB_BACKUP_UNIT} is unexpectedly active after deploy (DB dump must not run from deploy)"
  fi
  if is_active "${MAINTAIN_INCREMENTAL_UNIT}" || is_active "${MAINTAIN_FULL_UNIT}"; then
    die "source-comment maintenance is unexpectedly active after deploy"
  fi

  nginx -t || die "nginx -t failed"

  local health_url
  health_url="$(resolve_health_url)"
  log "GET ${health_url}"
  curl -fsS --max-time 10 "${health_url}" >/dev/null || die "local /health check failed (${health_url})"

  log "GET http://127.0.0.1/ (homepage via nginx)"
  local home_code
  home_code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 http://127.0.0.1/ || true)"
  if [[ "${home_code}" != "200" && "${home_code}" != "301" && "${home_code}" != "302" ]]; then
    die "homepage HTTP check failed (status=${home_code})"
  fi

  # Same API listener as /health. A missing column returns HTTP 500 here while
  # /health and the static homepage stay 200. Non-200 fails the deploy, which
  # rolls back installed artifacts when INSTALL_STARTED is set.
  local catalog_url catalog_code
  catalog_url="${health_url%/health}/api/v1/videos?limit=1"
  log "GET ${catalog_url}"
  catalog_code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 10 "${catalog_url}" || true)"
  if [[ "${catalog_code}" != "200" ]]; then
    die "catalog HTTP check failed (status=${catalog_code} url=${catalog_url})"
  fi

  prune_old_backups

  log ""
  log "previous commit: ${PREV_COMMIT}"
  log "deployed commit: ${TARGET_COMMIT}"
  log "backup:          ${BACKUP_DIR}"
  log "DEPLOYMENT SUCCEEDED"
  INSTALL_STARTED=0
}

main "$@"
