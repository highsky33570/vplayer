#!/usr/bin/env bash
# Pure helpers for VPlayer local health checks (testable without Docker/systemd).

# disk_status used_pct -> ok|warn|fail
# warn >= warn_pct (default 80), fail >= fail_pct (default 90)
disk_status() {
  local used_pct="$1"
  local warn_pct="${2:-80}"
  local fail_pct="${3:-90}"
  if ! [[ "${used_pct}" =~ ^[0-9]+$ ]]; then
    printf 'fail'
    return 0
  fi
  if (( used_pct >= fail_pct )); then
    printf 'fail'
  elif (( used_pct >= warn_pct )); then
    printf 'warn'
  else
    printf 'ok'
  fi
}

# age_sec empty/-1 = never; -2 = invalid/future; thresholds in seconds
# Returns: ok|warn|fail
freshness_status() {
  local age_sec="${1:-}"
  local warn_sec="$2"
  local fail_sec="$3"
  # never triggered, or unusable timestamp (must not look like a positive age)
  if [[ -z "${age_sec}" || "${age_sec}" == "-1" || "${age_sec}" == "-2" ]]; then
    printf 'warn'
    return 0
  fi
  if ! [[ "${age_sec}" =~ ^[0-9]+$ ]]; then
    printf 'warn'
    return 0
  fi
  if (( age_sec > fail_sec )); then
    printf 'fail'
  elif (( age_sec > warn_sec )); then
    printf 'warn'
  else
    printf 'ok'
  fi
}

# Convert /proc/uptime-style seconds (integer or fractional) to integer microseconds.
# Example: "762847.254703" -> 762847254703
proc_uptime_seconds_to_usec() {
  local sec="${1:-}"
  local whole frac

  if [[ -z "${sec}" ]]; then
    return 1
  fi
  whole="${sec%%.*}"
  if ! [[ "${whole}" =~ ^[0-9]+$ ]]; then
    return 1
  fi
  if [[ "${sec}" == *.* ]]; then
    frac="${sec#*.}"
    frac="${frac}000000"
    frac="${frac:0:6}"
    if ! [[ "${frac}" =~ ^[0-9]+$ ]]; then
      return 1
    fi
  else
    frac="000000"
  fi
  printf '%s' "$((10#${whole} * 1000000 + 10#${frac}))"
}

# Current Linux monotonic/uptime clock in microseconds.
# Prefers CLOCK_MONOTONIC via python3 when available (closest to systemd); else /proc/uptime.
linux_monotonic_now_usec() {
  local v line
  if command -v python3 >/dev/null 2>&1; then
    v="$(python3 -c 'import time; print(int(time.clock_gettime(time.CLOCK_MONOTONIC) * 1_000_000))' 2>/dev/null || true)"
    if [[ "${v}" =~ ^[0-9]+$ ]]; then
      printf '%s' "${v}"
      return 0
    fi
  fi
  line="$(awk '{print $1}' /proc/uptime 2>/dev/null || true)"
  proc_uptime_seconds_to_usec "${line}"
}

# Compute timer age from systemd LastTriggerUSecMonotonic (uint64 microseconds).
# now_usec: current monotonic/uptime microseconds (same unit).
# Prints:
#   -1  never triggered / unset (usec empty or 0)
#   -2  invalid or trigger > now (fail closed — never a false positive stale age)
#   >=0 age in whole seconds
#
# Does NOT parse human-readable systemd timestamps (avoids ambiguous CST/CDT/etc.).
timer_age_from_monotonic_usec() {
  local last_usec="${1:-}"
  local now_usec="${2:-}"

  if [[ -z "${last_usec}" || "${last_usec}" == "0" ]]; then
    printf '%s' "-1"
    return 0
  fi
  if ! [[ "${last_usec}" =~ ^[0-9]+$ ]] || ! [[ "${now_usec}" =~ ^[0-9]+$ ]]; then
    printf '%s' "-2"
    return 0
  fi
  if (( last_usec > now_usec )); then
    printf '%s' "-2"
    return 0
  fi
  printf '%s' "$(( (now_usec - last_usec) / 1000000 ))"
}

# Worker timer defaults (~3h / ~6h schedules + slack for RandomizedDelaySec).
# sync/enrich: warn >5h, fail >8h
# migrate: warn >10h, fail >15h
SYNC_WARN_SEC=$((5 * 3600))
SYNC_FAIL_SEC=$((8 * 3600))
ENRICH_WARN_SEC=$((5 * 3600))
ENRICH_FAIL_SEC=$((8 * 3600))
MIGRATE_WARN_SEC=$((10 * 3600))
MIGRATE_FAIL_SEC=$((15 * 3600))
BACKUP_MAX_AGE_SEC=$((36 * 3600))

# Combine line statuses into overall: fail > warn > ok
# Args: list of ok|warn|fail
overall_status() {
  local s worst=ok
  for s in "$@"; do
    case "${s}" in
      fail) worst=fail ;;
      warn) [[ "${worst}" != fail ]] && worst=warn ;;
      ok) ;;
      *) worst=fail ;;
    esac
  done
  printf '%s' "${worst}"
}

# Exit code: 0 for ok/warn, 1 for fail
overall_exit_code() {
  local status="$1"
  if [[ "${status}" == fail ]]; then
    return 1
  fi
  return 0
}

http_code_ok_for_homepage() {
  local code="$1"
  case "${code}" in
    200|301|302) return 0 ;;
    *) return 1 ;;
  esac
}
