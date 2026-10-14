#!/usr/bin/env bash
# Validates migratemedia systemd unit files (no systemctl / no secrets).
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SVC="${ROOT}/deploy/systemd/vplayer-migratemedia.service"
TIMER="${ROOT}/deploy/systemd/vplayer-migratemedia.timer"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf 'PASS: %s\n' "$*"; }

[[ -f "${SVC}" ]] || fail "missing ${SVC}"
[[ -f "${TIMER}" ]] || fail "missing ${TIMER}"

grep -q '^Type=oneshot$' "${SVC}" || fail "service must be Type=oneshot"
grep -q '^WorkingDirectory=/opt/vplayer/backend$' "${SVC}" || fail "WorkingDirectory"
grep -q '^EnvironmentFile=-/etc/vplayer/vplayer.env$' "${SVC}" || fail "EnvironmentFile"
grep -q '^ExecStart=/opt/vplayer/backend/bin/migratemedia --execute --confirm=MIGRATE --limit 1 --trigger=timer$' "${SVC}" \
  || fail "ExecStart must be exact migrate CLI"
grep -q '^TimeoutStartSec=35m$' "${SVC}" || fail "TimeoutStartSec=35m"
grep -q 'R2_ACCESS_KEY\|R2_SECRET\|password\|AKIA' "${SVC}" && fail "service must not embed secrets" || true
pass "service ExecStart / env / timeout"

grep -q '^OnCalendar=\*\-\*\-\* 00/6:40:00$' "${TIMER}" || fail "OnCalendar every 6h at :40"
grep -q '^Persistent=false$' "${TIMER}" || fail "Persistent=false (no catch-up on enable/deploy)"
grep -q '^RandomizedDelaySec=10min$' "${TIMER}" || fail "RandomizedDelaySec=10min"
grep -q '^OnBootSec=' "${TIMER}" && fail "must not set OnBootSec (avoid aggressive boot run)" || true
grep -q '^Unit=vplayer-migratemedia.service$' "${TIMER}" || fail "timer Unit="
pass "timer schedule"

# Deploy script must enable timer only, never start the oneshot service.
DEPLOY="${ROOT}/deploy/deploy-production.sh"
grep -q 'install_migratemedia_units' "${DEPLOY}" || fail "deploy missing install_migratemedia_units"
grep -q 'systemctl enable --now "${MIGRATE_TIMER}"' "${DEPLOY}" || fail "deploy must enable --now timer"
# Ignore comments when checking for an actual start of the oneshot.
if grep -E '^[[:space:]]*systemctl start ("\$\{MIGRATE_UNIT\}"|vplayer-migratemedia\.service)' "${DEPLOY}"; then
  fail "deploy must not start migratemedia service"
fi
pass "deploy enables timer only"

printf 'ALL migratemedia unit tests passed\n'
