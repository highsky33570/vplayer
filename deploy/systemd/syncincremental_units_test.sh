#!/usr/bin/env bash
# Contract tests for syncincremental + enrichpending systemd units.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
UNIT_DIR="${ROOT}/deploy/systemd"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf 'PASS: %s\n' "$*"; }

sync_svc="${UNIT_DIR}/vplayer-syncincremental.service"
sync_timer="${UNIT_DIR}/vplayer-syncincremental.timer"
enrich_svc="${UNIT_DIR}/vplayer-enrichpending.service"
enrich_timer="${UNIT_DIR}/vplayer-enrichpending.timer"

[[ -f "${sync_svc}" && -f "${sync_timer}" ]] || fail "missing syncincremental units"
[[ -f "${enrich_svc}" && -f "${enrich_timer}" ]] || fail "missing enrichpending units"

grep -q '^Type=oneshot$' "${sync_svc}" || fail "sync not oneshot"
grep -q '^WorkingDirectory=/opt/vplayer$' "${sync_svc}" || fail "sync WorkingDirectory"
grep -q '^ExecStart=/opt/vplayer/backend/bin/syncincremental --limit 15 --trigger=timer$' "${sync_svc}" || fail "sync ExecStart"
grep -q '^User=' "${sync_svc}" && fail "sync must not set User=" || true
grep -qiE 'OLEHDTV_AUTH_COOKIE|password=' "${sync_svc}" && fail "secrets in sync unit" || true

grep -q '^OnCalendar=\*-\*-\* 00/3:00:00$' "${sync_timer}" || fail "sync OnCalendar"
grep -q '^Unit=vplayer-syncincremental.service$' "${sync_timer}" || fail "sync timer Unit="
grep -qiE '^Requires=.*vplayer-syncincremental\.service' "${sync_timer}" && fail "sync timer must not Requires= oneshot" || true

grep -q '^Type=oneshot$' "${enrich_svc}" || fail "enrich not oneshot"
grep -q '^WorkingDirectory=/opt/vplayer$' "${enrich_svc}" || fail "enrich WorkingDirectory"
grep -q '^ExecStart=/opt/vplayer/backend/bin/enrichpending --limit 15 --concurrency 1 --trigger=timer$' "${enrich_svc}" || fail "enrich ExecStart"
grep -q '^User=' "${enrich_svc}" && fail "enrich must not set User=" || true

grep -q '^OnCalendar=\*-\*-\* 00/3:20:00$' "${enrich_timer}" || fail "enrich OnCalendar"
grep -qiE '^Requires=.*vplayer-enrichpending\.service' "${enrich_timer}" && fail "enrich timer must not Requires= oneshot" || true

pass "syncincremental/enrichpending systemd unit contracts"
printf 'ALL sync unit tests passed\n'
