#!/usr/bin/env bash
# VPlayer Plan 8: lightweight local health monitoring (no external alerts).
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/health_helpers.sh
source "${SCRIPT_DIR}/lib/health_helpers.sh"
# shellcheck source=lib/backup_helpers.sh
source "${SCRIPT_DIR}/lib/backup_helpers.sh"
# shellcheck source=lib/mysql_defaults.sh
source "${SCRIPT_DIR}/lib/mysql_defaults.sh"

readonly API_HEALTH_URL="${VPLAYER_HEALTH_API_URL:-http://127.0.0.1:8080/health}"
readonly HOME_URL="${VPLAYER_HEALTH_HOME_URL:-http://127.0.0.1/}"
readonly MYSQL_CONTAINER="${VPLAYER_MYSQL_CONTAINER:-vplayer-mysql}"
readonly DISK_PATH="${VPLAYER_HEALTH_DISK_PATH:-/opt/vplayer}"
readonly BACKUP_DIR="${VPLAYER_DB_BACKUP_DIR:-/opt/vplayer/backups/mysql}"
readonly STATUS_FILE="${VPLAYER_HEALTH_STATUS_FILE:-/var/lib/vplayer/health.status}"
STATUS_DIR="$(dirname "${STATUS_FILE}")"

declare -a LINE_STATUSES=()

log_line() {
  local status="$1"
  local component="$2"
  local detail="$3"
  local label
  LINE_STATUSES+=("${status}")
  case "${status}" in
    ok) label=PASS ;;
    warn) label=WARN ;;
    fail) label=FAIL ;;
    *) label=FAIL ;;
  esac
  printf '%s  %s  %s\n' "$(printf '%-4s' "${label}")" "${component}" "${detail}"
}

unit_is_active() {
  systemctl is-active --quiet "$1"
}

# Read LastTriggerUSecMonotonic as raw uint64 microseconds via busctl.
# Do NOT parse systemctl's human LastTriggerUSec (ambiguous CST / timespans).
# Do NOT use the Realtime trigger property name (absent on some production systemd builds).
timer_last_trigger_monotonic_usec() {
  local unit="$1"
  local path prop usec

  path="$(busctl call org.freedesktop.systemd1 /org/freedesktop/systemd1 \
    org.freedesktop.systemd1.Manager GetUnit s "${unit}" 2>/dev/null \
    | sed -n 's/^o "\(.*\)"$/\1/p')"
  if [[ -z "${path}" ]]; then
    return 1
  fi

  prop="$(busctl get-property org.freedesktop.systemd1 "${path}" \
    org.freedesktop.systemd1.Timer LastTriggerUSecMonotonic 2>/dev/null || true)"
  # Terse busctl form: "t 762847254703"
  usec="$(printf '%s\n' "${prop}" | awk '/^t / { print $2; exit }')"
  if [[ -z "${usec}" ]]; then
    return 1
  fi
  printf '%s' "${usec}"
}

timer_last_trigger_age_sec() {
  local unit="$1"
  local last_usec now_usec age

  last_usec="$(timer_last_trigger_monotonic_usec "${unit}" 2>/dev/null || true)"
  if [[ -z "${last_usec}" ]]; then
    # Missing busctl/property → unknown (WARN), distinct from successful read of 0 (never).
    printf '%s' "-2"
    return 0
  fi
  now_usec="$(linux_monotonic_now_usec 2>/dev/null || true)"
  if [[ -z "${now_usec}" ]]; then
    printf '%s' "-2"
    return 0
  fi
  age="$(timer_age_from_monotonic_usec "${last_usec}" "${now_usec}")"
  printf '%s' "${age}"
}

check_api() {
  local code
  code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 8 "${API_HEALTH_URL}" 2>/dev/null || echo 000)"
  if [[ "${code}" == "200" ]]; then
    log_line ok api "HTTP ${code} ${API_HEALTH_URL%%\?*}"
  else
    log_line fail api "HTTP ${code} (want 200)"
  fi
}

check_nginx_home() {
  local code
  code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 8 "${HOME_URL}" 2>/dev/null || echo 000)"
  if http_code_ok_for_homepage "${code}"; then
    log_line ok nginx "HTTP ${code} homepage"
  else
    log_line fail nginx "HTTP ${code} homepage (want 200/301/302)"
  fi
}

# Lightweight ping using container env + shared mysql_defaults helper.
mysql_container_ping() {
  {
    cat "${SCRIPT_DIR}/lib/mysql_defaults.sh"
    cat <<'EOS'
set -euo pipefail
conf=$(mktemp)
trap 'rm -f "$conf"' EXIT INT TERM HUP
write_mysql_client_defaults_file "$conf" "${MYSQL_USER}" "${MYSQL_PASSWORD}" || exit 1
mysqladmin --defaults-extra-file="$conf" ping --silent
EOS
  } | docker exec -i "${MYSQL_CONTAINER}" bash -s
}

check_mysql() {
  local st
  st="$(docker inspect -f '{{.State.Running}}' "${MYSQL_CONTAINER}" 2>/dev/null || true)"
  if [[ "${st}" != "true" ]]; then
    log_line fail mysql "container ${MYSQL_CONTAINER} not running"
    return 0
  fi
  if mysql_container_ping >/dev/null 2>&1; then
    log_line ok mysql "container running; ping ok"
  else
    log_line fail mysql "container running; ping failed"
  fi
}

check_disk() {
  local used
  used="$(df -P "${DISK_PATH}" 2>/dev/null | awk 'NR==2 {gsub(/%/,"",$5); print $5}')"
  if [[ -z "${used}" ]]; then
    log_line fail disk "unable to read usage for ${DISK_PATH}"
    return 0
  fi
  local st
  st="$(disk_status "${used}" 80 90)"
  case "${st}" in
    ok) log_line ok disk "${DISK_PATH} used=${used}%" ;;
    warn) log_line warn disk "${DISK_PATH} used=${used}% (>=80%)" ;;
    *) log_line fail disk "${DISK_PATH} used=${used}% (>=90%)" ;;
  esac
}

check_systemd_units() {
  local u
  for u in \
    vplayer-api.service \
    vplayer-syncincremental.timer \
    vplayer-enrichpending.timer \
    vplayer-migratemedia.timer \
    vplayer-db-backup.timer \
    vplayer-healthcheck.timer; do
    if unit_is_active "${u}"; then
      log_line ok systemd "${u} active"
    else
      log_line fail systemd "${u} not active"
    fi
  done
}

format_timer_age_detail() {
  local age="$1"
  local warn_sec="$2"
  case "${age}" in
    -1) printf 'never triggered (warn)' ;;
    -2) printf 'trigger timestamp unavailable/invalid (warn)' ;;
    *) printf 'last trigger age=%ss (warn>%ss or never/invalid)' "${age}" "${warn_sec}" ;;
  esac
}

check_timer_freshness() {
  local age st detail
  age="$(timer_last_trigger_age_sec vplayer-syncincremental.timer)"
  st="$(freshness_status "${age}" "${SYNC_WARN_SEC}" "${SYNC_FAIL_SEC}")"
  case "${st}" in
    ok) log_line ok timer.sync "last trigger age=${age}s" ;;
    warn)
      detail="$(format_timer_age_detail "${age}" "${SYNC_WARN_SEC}")"
      log_line warn timer.sync "${detail}"
      ;;
    *) log_line fail timer.sync "last trigger age=${age}s (fail>${SYNC_FAIL_SEC}s)" ;;
  esac

  age="$(timer_last_trigger_age_sec vplayer-enrichpending.timer)"
  st="$(freshness_status "${age}" "${ENRICH_WARN_SEC}" "${ENRICH_FAIL_SEC}")"
  case "${st}" in
    ok) log_line ok timer.enrich "last trigger age=${age}s" ;;
    warn)
      detail="$(format_timer_age_detail "${age}" "${ENRICH_WARN_SEC}")"
      log_line warn timer.enrich "${detail}"
      ;;
    *) log_line fail timer.enrich "last trigger age=${age}s (fail>${ENRICH_FAIL_SEC}s)" ;;
  esac

  age="$(timer_last_trigger_age_sec vplayer-migratemedia.timer)"
  st="$(freshness_status "${age}" "${MIGRATE_WARN_SEC}" "${MIGRATE_FAIL_SEC}")"
  case "${st}" in
    ok) log_line ok timer.migrate "last trigger age=${age}s" ;;
    warn)
      detail="$(format_timer_age_detail "${age}" "${MIGRATE_WARN_SEC}")"
      log_line warn timer.migrate "${detail}"
      ;;
    *) log_line fail timer.migrate "last trigger age=${age}s (fail>${MIGRATE_FAIL_SEC}s)" ;;
  esac
}

check_backup_freshness() {
  local st name age
  st="$(db_backup_freshness_status "${BACKUP_DIR}" "${BACKUP_MAX_AGE_SEC}")"
  name="$(newest_completed_db_backup "${BACKUP_DIR}")"
  case "${st}" in
    ok)
      age="$(db_backup_age_seconds "${BACKUP_DIR}/${name}")"
      log_line ok backup "newest=${name} age=${age}s"
      ;;
    warn_missing)
      log_line warn backup "no completed DB backup yet under ${BACKUP_DIR}"
      ;;
    fail_stale)
      age="$(db_backup_age_seconds "${BACKUP_DIR}/${name}")"
      log_line fail backup "newest=${name} age=${age}s (>${BACKUP_MAX_AGE_SEC}s)"
      ;;
    *)
      log_line fail backup "unknown freshness status"
      ;;
  esac
}

write_status_file() {
  local overall="$1"
  mkdir -p "${STATUS_DIR}"
  chmod 755 "${STATUS_DIR}" 2>/dev/null || true
  local tmp
  tmp="$(mktemp "${STATUS_DIR}/.health.XXXXXX")"
  {
    printf 'overall=%s\n' "${overall}"
    printf 'checked_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  } >"${tmp}"
  chmod 644 "${tmp}"
  mv -f "${tmp}" "${STATUS_FILE}"
}

main() {
  printf 'VPlayer healthcheck\n'
  check_api
  check_nginx_home
  check_mysql
  check_disk
  check_systemd_units
  check_timer_freshness
  check_backup_freshness

  local overall overall_label
  overall="$(overall_status "${LINE_STATUSES[@]}")"
  case "${overall}" in
    ok) overall_label=PASS ;;
    warn) overall_label=WARN ;;
    *) overall_label=FAIL ;;
  esac
  printf 'OVERALL %s\n' "${overall_label}"
  write_status_file "${overall}" || true

  if overall_exit_code "${overall}"; then
    exit 0
  fi
  exit 1
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  main "$@"
fi
