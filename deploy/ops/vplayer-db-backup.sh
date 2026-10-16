#!/usr/bin/env bash
# VPlayer Plan 8: full logical MySQL backup (gzip, atomic, flock, retention).
# Intended to run on the production host as root via systemd oneshot.
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/backup_helpers.sh
source "${SCRIPT_DIR}/lib/backup_helpers.sh"
# shellcheck source=lib/mysql_defaults.sh
source "${SCRIPT_DIR}/lib/mysql_defaults.sh"

readonly BACKUP_ROOT="${VPLAYER_DB_BACKUP_DIR:-/opt/vplayer/backups/mysql}"
readonly MYSQL_CONTAINER="${VPLAYER_MYSQL_CONTAINER:-vplayer-mysql}"
readonly RETAIN="${VPLAYER_DB_BACKUP_RETAIN:-14}"
readonly LOCK_FILE="${VPLAYER_DB_BACKUP_LOCK:-/run/vplayer-db-backup.lock}"

# Host-side path of the in-progress .partial (never a completed .sql.gz).
CURRENT_PARTIAL=""

log() { printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
err() { printf '%s ERROR: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2; }

die() {
  err "$*"
  exit 1
}

cleanup_current_partial() {
  local p="${CURRENT_PARTIAL:-}"
  CURRENT_PARTIAL=""
  # Only remove in-progress partials — never completed backups.
  if [[ -n "${p}" && -e "${p}" && "${p}" == *.partial ]]; then
    rm -f "${p}" || true
  fi
}

ensure_backup_dir() {
  mkdir -p "${BACKUP_ROOT}"
  chmod 700 "${BACKUP_ROOT}"
  local parent
  parent="$(dirname "${BACKUP_ROOT}")"
  if [[ -d "${parent}" ]]; then
    chmod 700 "${parent}" 2>/dev/null || true
  fi
}

container_running() {
  local st
  st="$(docker inspect -f '{{.State.Running}}' "${MYSQL_CONTAINER}" 2>/dev/null || true)"
  [[ "${st}" == "true" ]]
}

# Dump using credentials already present in the MySQL container environment.
# Security tradeoff:
#   - We do NOT read or source /opt/vplayer/.env on the host.
#   - We do NOT print credentials.
#   - Inside the container, write_mysql_client_defaults_file builds a mode-600
#     --defaults-extra-file with MySQL option-file escaping; mysqldump never
#     receives the password on argv. Helper is piped into `bash -s` (library
#     text only — no secret values on the host docker argv).
run_mysqldump_to_stdout() {
  {
    cat "${SCRIPT_DIR}/lib/mysql_defaults.sh"
    cat <<'EOS'
set -euo pipefail
conf=$(mktemp)
trap 'rm -f "$conf"' EXIT INT TERM HUP
write_mysql_client_defaults_file "$conf" "${MYSQL_USER}" "${MYSQL_PASSWORD}" || {
  echo "ERROR: refused unsafe MySQL credential characters (CR/LF)" >&2
  exit 1
}
mysqldump --defaults-extra-file="$conf" \
  --single-transaction --routines --triggers --no-tablespaces \
  --default-character-set=utf8mb4 \
  "${MYSQL_DATABASE}"
EOS
  } | docker exec -i "${MYSQL_CONTAINER}" bash -s
}

perform_backup() {
  ensure_backup_dir
  container_running || die "MySQL container ${MYSQL_CONTAINER} is not running"

  local base partial final sz
  base="$(make_db_backup_basename)"
  partial="${BACKUP_ROOT}/${base}${VPLAYER_DB_BACKUP_PARTIAL_SUFFIX}"
  final="${BACKUP_ROOT}/${base}"

  rm -f "${partial}"
  CURRENT_PARTIAL="${partial}"
  log "starting backup container=${MYSQL_CONTAINER} dest=${final}"

  # Stream dump → gzip into partial; only rename after success + validation.
  if ! run_mysqldump_to_stdout | gzip -c >"${partial}"; then
    cleanup_current_partial
    die "mysqldump|gzip failed"
  fi

  if ! sz="$(validate_backup_partial_for_finalize "${partial}" 64)"; then
    cleanup_current_partial
    die "backup validation failed (min size or gzip -t); refusing to finalize"
  fi

  chmod 600 "${partial}"
  mv -f "${partial}" "${final}"
  CURRENT_PARTIAL=""
  chmod 600 "${final}"
  log "backup completed bytes=${sz} file=${base}"

  local deleted
  deleted="$(prune_db_backups "${BACKUP_ROOT}" "${RETAIN}" || true)"
  if [[ -n "${deleted}" ]]; then
    while IFS= read -r line; do
      [[ -n "${line}" ]] && log "pruned ${line}"
    done <<<"${deleted}"
  fi
  log "retention keep=${RETAIN} dir=${BACKUP_ROOT}"
}

main() {
  if [[ "${EUID}" -ne 0 ]]; then
    die "must run as root"
  fi

  trap cleanup_current_partial EXIT INT TERM HUP

  mkdir -p "$(dirname "${LOCK_FILE}")"
  exec 9>"${LOCK_FILE}"
  if ! flock -n 9; then
    die "another backup is already running (lock ${LOCK_FILE})"
  fi

  perform_backup
  log "OK"
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
