#!/usr/bin/env bash
# Contract tests for Plan 8 systemd units (no systemd runtime required).
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
UNIT_DIR="${ROOT}/deploy/systemd"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf 'PASS: %s\n' "$*"; }

backup_svc="${UNIT_DIR}/vplayer-db-backup.service"
backup_timer="${UNIT_DIR}/vplayer-db-backup.timer"
health_svc="${UNIT_DIR}/vplayer-healthcheck.service"
health_timer="${UNIT_DIR}/vplayer-healthcheck.timer"

[[ -f "${backup_svc}" && -f "${backup_timer}" ]] || fail "missing db-backup units"
[[ -f "${health_svc}" && -f "${health_timer}" ]] || fail "missing healthcheck units"

grep -q '^Type=oneshot$' "${backup_svc}" || fail "db-backup not oneshot"
grep -q '^ExecStart=/opt/vplayer/deploy/ops/vplayer-db-backup.sh$' "${backup_svc}" || fail "db-backup ExecStart"
grep -q '^User=' "${backup_svc}" && fail "db-backup must not set User=" || true
grep -q 'MYSQL_PASSWORD\|DB_PASSWORD\|password=' "${backup_svc}" && fail "secrets in db-backup unit" || true

grep -q '^OnCalendar=\*-\*-\* 03:30:00$' "${backup_timer}" || fail "backup OnCalendar"
grep -q '^RandomizedDelaySec=45min$' "${backup_timer}" || fail "backup RandomizedDelaySec"
grep -q '^Unit=vplayer-db-backup.service$' "${backup_timer}" || fail "backup timer Unit="
grep -qiE '^Requires=.*vplayer-db-backup\.service' "${backup_timer}" && fail "timer must not Requires= oneshot" || true

grep -q '^Type=oneshot$' "${health_svc}" || fail "health not oneshot"
grep -q '^ExecStart=/opt/vplayer/deploy/ops/vplayer-healthcheck.sh$' "${health_svc}" || fail "health ExecStart"
grep -q '^SuccessExitStatus=' "${health_svc}" && fail "health must not treat FAIL as success" || true
grep -q '^User=' "${health_svc}" && fail "health must not set User=" || true

grep -q '^OnCalendar=\*:0/15:00$' "${health_timer}" || fail "health OnCalendar"
grep -q '^RandomizedDelaySec=2min$' "${health_timer}" || fail "health RandomizedDelaySec"
grep -q '^Unit=vplayer-healthcheck.service$' "${health_timer}" || fail "health timer Unit="

pass "Plan 8 systemd unit contracts"
printf 'ALL Plan 8 unit tests passed\n'
