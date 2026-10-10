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
readonly FRONTEND_DIST="${PROD_ROOT}/frontend/dist"
readonly BACKUP_ROOT="${PROD_ROOT}/releases/backups"
readonly MAX_BACKUPS=5

readonly API_UNIT="vplayer-api.service"
readonly SYNC_UNIT="vplayer-syncincremental.service"
readonly ENRICH_UNIT="vplayer-enrichpending.service"
readonly SYNC_TIMER="vplayer-syncincremental.timer"
readonly ENRICH_TIMER="vplayer-enrichpending.timer"

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

  log "backend: build cmd/api, syncincremental, enrichpending"
  (
    cd "${REPO_DIR}/backend"
    go build -o "${STAGING}/bin/vplayer-api" ./cmd/api
    go build -o "${STAGING}/bin/syncincremental" ./cmd/syncincremental
    go build -o "${STAGING}/bin/enrichpending" ./cmd/enrichpending
  )

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

  # --- Backups (only after all builds succeed) ---
  log "creating rollback backup under ${BACKUP_DIR}"
  mkdir -p "${BACKUP_DIR}"
  backup_path_if_exists "${API_BIN}" "vplayer-api"
  backup_path_if_exists "${SYNC_BIN}" "syncincremental"
  backup_path_if_exists "${ENRICH_BIN}" "enrichpending"
  if [[ -d "${FRONTEND_DIST}" ]]; then
    cp -a "${FRONTEND_DIST}" "${BACKUP_DIR}/frontend-dist"
  fi

  # --- Install (production mutation begins) ---
  INSTALL_STARTED=1
  log "installing binaries (atomic rename)…"
  mkdir -p "${WORKER_BIN_DIR}" "$(dirname "${API_BIN}")" "$(dirname "${FRONTEND_DIST}")"

  atomic_install_bin "${STAGING}/bin/vplayer-api" "${API_BIN}"
  atomic_install_bin "${STAGING}/bin/syncincremental" "${SYNC_BIN}"
  atomic_install_bin "${STAGING}/bin/enrichpending" "${ENRICH_BIN}"

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

  log "restarting ${API_UNIT} only (timers/workers untouched)…"
  systemctl restart "${API_UNIT}"
  sleep 1

  # --- Verify ---
  log "verifying units and endpoints…"
  is_active "${API_UNIT}" || die "${API_UNIT} is not active after restart"
  is_active "${SYNC_TIMER}" || die "${SYNC_TIMER} is not active"
  is_active "${ENRICH_TIMER}" || die "${ENRICH_TIMER} is not active"

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

  prune_old_backups

  log ""
  log "previous commit: ${PREV_COMMIT}"
  log "deployed commit: ${TARGET_COMMIT}"
  log "backup:          ${BACKUP_DIR}"
  log "DEPLOYMENT SUCCEEDED"
  INSTALL_STARTED=0
}

main "$@"
