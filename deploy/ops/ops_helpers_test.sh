#!/usr/bin/env bash
# Plan 8 helper tests (no Docker/systemd/production secrets).
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=lib/backup_helpers.sh
source "${ROOT}/deploy/ops/lib/backup_helpers.sh"
# shellcheck source=lib/health_helpers.sh
source "${ROOT}/deploy/ops/lib/health_helpers.sh"
# shellcheck source=lib/mysql_defaults.sh
source "${ROOT}/deploy/ops/lib/mysql_defaults.sh"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf 'PASS: %s\n' "$*"; }

TMPROOT="$(mktemp -d)"
trap 'rm -rf "${TMPROOT}"' EXIT

# --- backup naming / partial rejection ---
is_recognized_db_backup_name "vplayer-mysql-20260415T031500Z.sql.gz" || fail "valid name rejected"
is_recognized_db_backup_name "vplayer-mysql-20260415T031500Z.sql.gz.partial" && fail "partial accepted as completed"
is_recognized_db_backup_name "evil.sql.gz" && fail "unrecognized name accepted"
is_partial_db_backup_name "vplayer-mysql-20260415T031500Z.sql.gz.partial" || fail "partial suffix not detected"
pass "backup filename recognition"

# --- MySQL option-file escaping (no secret printing) ---
q="$(mysql_option_quote_value 'plain')"
[[ "${q}" == '"plain"' ]] || fail "plain quote"
q="$(mysql_option_quote_value 'spa ce')"
[[ "${q}" == '"spa ce"' ]] || fail "space quote"
q="$(mysql_option_quote_value 'a#b;c=d$e')"
[[ "${q}" == '"a#b;c=d$e"' ]] || fail "specials quote"
q="$(mysql_option_quote_value 'x"y\z')"
[[ "${q}" == '"x\"y\\z"' ]] || fail "quote/backslash escape"
mysql_option_value_is_safe $'line\nfeed' && fail "LF should be rejected"
mysql_option_value_is_safe $'line\rfeed' && fail "CR should be rejected"
mysql_option_quote_value $'bad\n' && fail "quote LF should fail" || true
mysql_option_quote_value $'bad\r' && fail "quote CR should fail" || true

defaults_path="${TMPROOT}/client.cnf"
# Fixture credentials are non-production test values only.
write_mysql_client_defaults_file "${defaults_path}" 'u#ser' 'p@ss #;x="y"' || fail "write defaults"
[[ -f "${defaults_path}" ]] || fail "defaults missing"
mode="$(stat -c '%a' "${defaults_path}" 2>/dev/null || stat -f '%OLp' "${defaults_path}" 2>/dev/null || echo "")"
[[ "${mode}" == "600" ]] || fail "defaults mode want 600 got ${mode}"
grep -q '^\[client\]$' "${defaults_path}" || fail "section"
grep -q '^user="u#ser"$' "${defaults_path}" || fail "user line"
grep -q '^password="p@ss #;x=\\"y\\""$' "${defaults_path}" || fail "password line escaped"
# Dollar + backslash covered by quote_value assertions above (not logged).
if write_mysql_client_defaults_file "${TMPROOT}/bad.cnf" $'a\nb' 'ok'; then
  fail "LF user must fail closed"
fi
if write_mysql_client_defaults_file "${TMPROOT}/bad2.cnf" 'ok' $'a\rb'; then
  fail "CR password must fail closed"
fi
pass "mysql option-file escaping / reject CR-LF / mode 600"

# --- retention keeps actual newest 14 ---
ret_dir="${TMPROOT}/retention"
mkdir -p "${ret_dir}"
# Oldest two: ...T120100Z and ...T120200Z — must be deleted
for i in $(seq -w 1 16); do
  path="${ret_dir}/vplayer-mysql-20260101T12${i}00Z.sql.gz"
  printf 'x' >"${path}"
  touch -t "2026010112${i}" "${path}" 2>/dev/null || true
done
printf 'x' >"${ret_dir}/not-a-backup.sql.gz"
printf 'x' >"${ret_dir}/vplayer-mysql-20260101T999999Z.sql.gz.partial"
printf 'x' >"${ret_dir}/vplayer-mysql-NOTIME.sql.gz"

prune_db_backups "${ret_dir}" 14 >/dev/null
[[ ! -f "${ret_dir}/vplayer-mysql-20260101T120100Z.sql.gz" ]] || fail "oldest should be pruned"
[[ ! -f "${ret_dir}/vplayer-mysql-20260101T120200Z.sql.gz" ]] || fail "second oldest should be pruned"
[[ -f "${ret_dir}/vplayer-mysql-20260101T120300Z.sql.gz" ]] || fail "3rd oldest should remain"
[[ -f "${ret_dir}/vplayer-mysql-20260101T121600Z.sql.gz" ]] || fail "newest should remain"
remain=0
for f in "${ret_dir}"/vplayer-mysql-*.sql.gz; do
  [[ -f "${f}" ]] || continue
  base="$(basename "${f}")"
  is_recognized_db_backup_name "${base}" && remain=$((remain + 1))
done
[[ "${remain}" -eq 14 ]] || fail "expected 14 recognized backups, got ${remain}"
[[ -f "${ret_dir}/not-a-backup.sql.gz" ]] || fail "decoy was deleted"
[[ -f "${ret_dir}/vplayer-mysql-20260101T999999Z.sql.gz.partial" ]] || fail "partial was deleted by prune"
[[ -f "${ret_dir}/vplayer-mysql-NOTIME.sql.gz" ]] || fail "malformed was deleted by prune"
pass "retention keeps newest 14 recognized only"

# --- freshness: ignore partial/malformed; stale >36h = fail_stale ---
fresh_dir="${TMPROOT}/fresh"
mkdir -p "${fresh_dir}"
[[ "$(db_backup_freshness_status "${fresh_dir}" 129600)" == "warn_missing" ]] || fail "empty warn_missing"
printf 'x' >"${fresh_dir}/vplayer-mysql-20260101T120000Z.sql.gz.partial"
printf 'x' >"${fresh_dir}/vplayer-mysql-NOTIME.sql.gz"
printf 'x' >"${fresh_dir}/junk.sql.gz"
[[ "$(db_backup_freshness_status "${fresh_dir}" 129600)" == "warn_missing" ]] || fail "partial/malformed must not count"
# Deterministic stale age via helper math: create completed + force mtime epoch
stale_name="vplayer-mysql-20200101T000000Z.sql.gz"
printf 'x' >"${fresh_dir}/${stale_name}"
# 40 hours ago in seconds from now
if touch -d '40 hours ago' "${fresh_dir}/${stale_name}" 2>/dev/null; then
  :
elif touch -t 202001010000 "${fresh_dir}/${stale_name}" 2>/dev/null; then
  :
else
  fail "unable to set stale mtime for freshness test"
fi
age="$(db_backup_age_seconds "${fresh_dir}/${stale_name}")"
[[ -n "${age}" && "${age}" -gt 129600 ]] || fail "stale fixture age not >36h (age=${age})"
[[ "$(db_backup_freshness_status "${fresh_dir}" 129600)" == "fail_stale" ]] || fail "stale >36h must be fail_stale"
# Fresh completed wins
printf 'y' >"${fresh_dir}/vplayer-mysql-20990101T120000Z.sql.gz"
touch "${fresh_dir}/vplayer-mysql-20990101T120000Z.sql.gz"
[[ "$(db_backup_freshness_status "${fresh_dir}" 129600)" == "ok" ]] || fail "fresh backup should be ok"
pass "backup freshness ignore partial/malformed + stale FAIL"

# --- gzip validation before finalize ---
gz_dir="${TMPROOT}/gz"
mkdir -p "${gz_dir}"
# Low-compressibility payload so gzip output is still >= 64 bytes
i=0
while (( i < 4000 )); do
  # Varying bytes defeat tiny gzip headers-only outputs
  printf '%s' "${i}-abcdefghijklmnopqrstuvwxyz-"
  i=$((i + 1))
done | gzip -c >"${gz_dir}/ok.sql.gz.partial"
sz="$(validate_backup_partial_for_finalize "${gz_dir}/ok.sql.gz.partial" 64)" || fail "valid gzip should pass"
[[ "${sz}" -ge 64 ]] || fail "size printed"
# Too small but valid gzip
printf 'x' | gzip -c >"${gz_dir}/tiny.sql.gz.partial"
validate_backup_partial_for_finalize "${gz_dir}/tiny.sql.gz.partial" 64 && fail "tiny gzip should fail min size" || true
# Corrupt / not gzip, padded to >= 64
: >"${gz_dir}/bad.sql.gz.partial"
i=0
while (( i < 80 )); do printf 'N' >>"${gz_dir}/bad.sql.gz.partial"; i=$((i + 1)); done
validate_backup_partial_for_finalize "${gz_dir}/bad.sql.gz.partial" 64 && fail "corrupt gzip should fail" || true
# Contract: backup script must call validator (implies gzip -t path)
grep -q 'validate_backup_partial_for_finalize' "${ROOT}/deploy/ops/vplayer-db-backup.sh" || fail "backup must validate partial"
grep -q 'gzip -t' "${ROOT}/deploy/ops/lib/backup_helpers.sh" || fail "helper must gzip -t"
pass "gzip validation helper + backup contract"

# --- disk / timer / overall ---
[[ "$(disk_status 10 80 90)" == "ok" ]] || fail "disk 10"
[[ "$(disk_status 80 80 90)" == "warn" ]] || fail "disk 80"
[[ "$(disk_status 90 80 90)" == "fail" ]] || fail "disk 90"
pass "disk thresholds"

[[ "$(freshness_status 100 3600 7200)" == "ok" ]] || fail "fresh ok"
[[ "$(freshness_status 4000 3600 7200)" == "warn" ]] || fail "fresh warn"
[[ "$(freshness_status 8000 3600 7200)" == "fail" ]] || fail "fresh fail"
[[ "$(freshness_status -1 3600 7200)" == "warn" ]] || fail "never warn"
[[ "$(freshness_status -2 3600 7200)" == "warn" ]] || fail "invalid/future warn"
[[ "$(freshness_status '-48714' 3600 7200)" == "warn" ]] || fail "negative age must not be ok/fail-stale"
pass "timer freshness helper"

# --- timer age from monotonic usec (production: LastTriggerUSecMonotonic) ---
# Production evidence: LastTriggerUSecRealtime does not exist; use Monotonic uint64 usec.
# Do not parse human LastTriggerUSec ("… CST") or systemctl timespans ("1w 1d 19h…").
[[ "$(proc_uptime_seconds_to_usec '762847.254703')" == "762847254703" ]] || fail "uptime→usec"
[[ "$(proc_uptime_seconds_to_usec '10')" == "10000000" ]] || fail "uptime integer"
now_mono=762847254703
# 1 hour earlier
last_1h=$(( now_mono - 3600 * 1000000 ))
[[ "$(timer_age_from_monotonic_usec "${last_1h}" "${now_mono}")" == "3600" ]] || fail "1h age"
[[ "$(timer_age_from_monotonic_usec 0 "${now_mono}")" == "-1" ]] || fail "usec 0 = never"
[[ "$(timer_age_from_monotonic_usec "" "${now_mono}")" == "-1" ]] || fail "empty usec = never"
[[ "$(timer_age_from_monotonic_usec "$((now_mono + 1))" "${now_mono}")" == "-2" ]] || fail "future = -2"
[[ "$(timer_age_from_monotonic_usec "not-a-number" "${now_mono}")" == "-2" ]] || fail "bad usec = -2"
[[ "$(timer_age_from_monotonic_usec 'Fri 2026-10-16 09:01:53 CST' "${now_mono}")" == "-2" ]] || fail "CST string rejected"
[[ "$(timer_age_from_monotonic_usec '1w 1d 19h 54min 7.254703s' "${now_mono}")" == "-2" ]] || fail "timespan rejected"
HC="${ROOT}/deploy/ops/vplayer-healthcheck.sh"
grep -q 'LastTriggerUSecMonotonic' "${HC}" || fail "must use LastTriggerUSecMonotonic"
if grep -qE 'LastTriggerUSecRealtime|timer_age_from_realtime_usec|timer_last_trigger_realtime' "${HC}"; then
  fail "must not reference Realtime trigger property/helpers"
fi
grep -q 'timer_age_from_monotonic_usec' "${HC}" || fail "must use monotonic age helper"
grep -q 'linux_monotonic_now_usec' "${HC}" || fail "must sample monotonic/uptime now"
grep -q 'busctl' "${HC}" || fail "must read raw usec via busctl"
if grep -qE 'date -d' "${HC}"; then
  fail "healthcheck must not date -d parse timestamps"
fi
if grep -qE 'systemctl show.*LastTriggerUSec' "${HC}"; then
  fail "must not systemctl show LastTriggerUSec"
fi
# get-property must target Monotonic only (not bare LastTriggerUSec)
if grep -E 'get-property.*LastTriggerUSec([^M]|$)' "${HC}"; then
  fail "must not get-property LastTriggerUSec (use Monotonic only)"
fi
pass "timer age monotonic usec / CST regression"

[[ "$(overall_status ok ok warn)" == "warn" ]] || fail "overall warn"
[[ "$(overall_status ok fail warn)" == "fail" ]] || fail "overall fail"
[[ "$(overall_status ok ok)" == "ok" ]] || fail "overall ok"
overall_exit_code ok || fail "exit ok"
overall_exit_code warn || fail "exit warn should be 0"
if overall_exit_code fail; then fail "exit fail should be 1"; fi
pass "overall status/exit"

http_code_ok_for_homepage 200 || fail "home 200"
http_code_ok_for_homepage 302 || fail "home 302"
http_code_ok_for_homepage 500 && fail "home 500" || true
pass "homepage codes"

# --- deploy / signal cleanup / shared helper contracts ---
DEPLOY="${ROOT}/deploy/deploy-production.sh"
grep -q 'install_plan8_ops_units' "${DEPLOY}" || fail "deploy missing install_plan8_ops_units"
grep -q 'mysql_defaults.sh' "${DEPLOY}" || fail "deploy must install mysql_defaults.sh"
grep -q 'DB_BACKUP_TIMER' "${DEPLOY}" || fail "deploy missing DB_BACKUP_TIMER"
grep -q 'HEALTH_TIMER' "${DEPLOY}" || fail "deploy missing HEALTH_TIMER"
if grep -E 'systemctl (start|restart|enable --now) ("\$\{DB_BACKUP_UNIT\}"|vplayer-db-backup\.service)' "${DEPLOY}"; then
  fail "deploy must not start db-backup oneshot"
fi
grep -q 'cleanup_current_partial' "${ROOT}/deploy/ops/vplayer-db-backup.sh" || fail "host partial cleanup missing"
grep -q 'EXIT INT TERM HUP' "${ROOT}/deploy/ops/vplayer-db-backup.sh" || fail "backup signal trap"
grep -q 'write_mysql_client_defaults_file' "${ROOT}/deploy/ops/vplayer-db-backup.sh" || fail "backup uses shared defaults writer"
grep -q 'write_mysql_client_defaults_file' "${ROOT}/deploy/ops/vplayer-healthcheck.sh" || fail "health uses shared defaults writer"
grep -q 'mysql -p\|mysqldump -p' "${ROOT}/deploy/ops/vplayer-db-backup.sh" && fail "password on argv style" || true
grep -q '<STAGING_MYSQL_CONTAINER>' "${ROOT}/docs/BACKUP_RESTORE.md" || fail "docs staging placeholder"
grep -q 'PROD_MYSQL_CONTAINER' "${ROOT}/docs/BACKUP_RESTORE.md" || fail "docs production target var"
grep -qE 'mysql --defaults-extra-file=<\(|bash -c.*<\(' "${ROOT}/docs/BACKUP_RESTORE.md" && fail "docs must not use process substitution in commands" || true
# Flag style password-on-argv in fenced commands (allow prose forbidding it).
if grep -E '^\s*(mysql|mysqldump).*-p[^ -]' "${ROOT}/docs/BACKUP_RESTORE.md"; then
  fail "docs must not show password-on-argv mysql flags"
fi
pass "deploy/systemd/docs contracts"

printf 'ALL Plan 8 ops helper tests passed\n'
