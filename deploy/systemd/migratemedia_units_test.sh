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
grep -q '^WorkingDirectory=/opt/vplayer$' "${SVC}" || fail "WorkingDirectory must be /opt/vplayer (dotenv .env)"
grep -q '^User=' "${SVC}" && fail "must not set User= (no vplayer Linux user)" || true
grep -q '^Group=' "${SVC}" && fail "must not set Group=" || true
grep -q '^EnvironmentFile=' "${SVC}" && fail "must not set EnvironmentFile= (use app dotenv)" || true
grep -q '^Environment=' "${SVC}" && fail "must not set Environment= (unnecessary for migratemedia)" || true
grep -q '^After=network-online.target docker.service$' "${SVC}" || fail "After= must match production sync worker"
grep -q '^ExecStart=/opt/vplayer/backend/bin/migratemedia --execute --confirm=MIGRATE --limit 1 --trigger=timer$' "${SVC}" \
  || fail "ExecStart must be exact migrate CLI"
grep -q '^TimeoutStartSec=35m$' "${SVC}" || fail "TimeoutStartSec=35m"
grep -q 'R2_ACCESS_KEY\|R2_SECRET\|password\|AKIA' "${SVC}" && fail "service must not embed secrets" || true
pass "service ExecStart / cwd / no User / timeout"

grep -q '^OnCalendar=\*\-\*\-\* 00/6:40:00$' "${TIMER}" || fail "OnCalendar every 6h at :40"
grep -q '^Persistent=false$' "${TIMER}" || fail "Persistent=false"
grep -q '^RandomizedDelaySec=10min$' "${TIMER}" || fail "RandomizedDelaySec=10min"
grep -q '^OnBootSec=' "${TIMER}" && fail "must not set OnBootSec" || true
grep -q '^Unit=vplayer-migratemedia.service$' "${TIMER}" || fail "timer Unit="
grep -q '^Requires=vplayer-migratemedia.service$' "${TIMER}" && fail "timer must not Requires= oneshot (would start migration with timer)" || true
pass "timer schedule / no Requires oneshot"

DEPLOY="${ROOT}/deploy/deploy-production.sh"
grep -q 'install_migratemedia_units' "${DEPLOY}" || fail "deploy missing install_migratemedia_units"
grep -q 'systemctl enable "${MIGRATE_TIMER}"' "${DEPLOY}" || fail "deploy must enable timer (without --now)"
grep -q 'systemctl restart "${MIGRATE_TIMER}"' "${DEPLOY}" || fail "deploy must restart timer only"
if grep -q 'enable --now "${MIGRATE_TIMER}"' "${DEPLOY}"; then
  fail "deploy must not enable --now timer"
fi
if grep -E '^[[:space:]]*systemctl (start|restart|enable --now) ("\$\{MIGRATE_UNIT\}"|vplayer-migratemedia\.service)' "${DEPLOY}"; then
  fail "deploy must not start/restart migratemedia service"
fi
pass "deploy enables/restarts timer only"

printf 'ALL migratemedia unit tests passed\n'
