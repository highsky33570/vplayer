#!/usr/bin/env bash
# Validates deploy-production.sh includes syncleafmembership binary install
# without systemd unit/timer or automatic execution (no systemctl / no secrets).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEPLOY="${ROOT}/deploy/deploy-production.sh"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

[[ -f "${DEPLOY}" ]] || fail "missing ${DEPLOY}"

grep -q 'readonly LEAF_BIN=' "${DEPLOY}" || fail "missing LEAF_BIN"
grep -q 'go build -o "${STAGING}/bin/syncleafmembership" ./cmd/syncleafmembership' "${DEPLOY}" \
  || fail "missing syncleafmembership go build"
grep -q 'atomic_install_bin "${STAGING}/bin/syncleafmembership" "${LEAF_BIN}"' "${DEPLOY}" \
  || fail "missing syncleafmembership atomic install"
grep -q 'backup_path_if_exists "${LEAF_BIN}" "syncleafmembership"' "${DEPLOY}" \
  || fail "missing syncleafmembership backup"
grep -q 'BACKUP_DIR}/syncleafmembership' "${DEPLOY}" || fail "missing syncleafmembership rollback restore"

# Must not auto-run or schedule leaf membership.
if grep -E 'syncleafmembership\.(service|timer)' "${DEPLOY}"; then
  fail "deploy must not install syncleafmembership systemd units"
fi
if grep -E 'ExecStart=.*syncleafmembership|systemctl .*(start|restart|enable).*syncleafmembership' "${DEPLOY}"; then
  fail "deploy must not start/enable syncleafmembership"
fi
# Deploy must not invoke the CLI during install.
if grep -E '[^[:alnum:]_]syncleafmembership[[:space:]]+--' "${DEPLOY}"; then
  fail "deploy must not execute syncleafmembership with flags"
fi

# Worker guard must include leaf binary.
grep -q 'worker_bins_running "${SYNC_BIN}" "${ENRICH_BIN}" "${MIGRATE_BIN}" "${LEAF_BIN}"' "${DEPLOY}" \
  || fail "worker_bins_running must include LEAF_BIN"

# Migrations 008/009 must exist in repo (applied via normal API startup path).
[[ -f "${ROOT}/backend/migrations/008_category_parent_id.sql" ]] || fail "missing migration 008"
[[ -f "${ROOT}/backend/migrations/009_video_leaf_source_id.sql" ]] || fail "missing migration 009"

# Ordering in deploy: migrations installed before API restart.
mig_line="$(grep -n 'atomic_install_migrations' "${DEPLOY}" | head -1 | cut -d: -f1)"
api_line="$(grep -n 'systemctl restart "${API_UNIT}"' "${DEPLOY}" | head -1 | cut -d: -f1)"
[[ -n "${mig_line}" && -n "${api_line}" ]] || fail "could not locate migration/API restart lines"
[[ "${mig_line}" -lt "${api_line}" ]] || fail "migrations must be installed before API restart"

printf 'ALL syncleafmembership deploy tests passed\n'
