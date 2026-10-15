#!/usr/bin/env bash
# Validates deploy-production.sh includes syncleafimport binary install
# without systemd unit/timer or automatic execution (no systemctl / no secrets).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEPLOY="${ROOT}/deploy/deploy-production.sh"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

[[ -f "${DEPLOY}" ]] || fail "missing ${DEPLOY}"

grep -q 'readonly LEAF_IMPORT_BIN=' "${DEPLOY}" || fail "missing LEAF_IMPORT_BIN"
grep -q 'go build -o "${STAGING}/bin/syncleafimport" ./cmd/syncleafimport' "${DEPLOY}" \
  || fail "missing syncleafimport go build"
grep -q 'atomic_install_bin "${STAGING}/bin/syncleafimport" "${LEAF_IMPORT_BIN}"' "${DEPLOY}" \
  || fail "missing syncleafimport atomic install"
grep -q 'backup_path_if_exists "${LEAF_IMPORT_BIN}" "syncleafimport"' "${DEPLOY}" \
  || fail "missing syncleafimport backup"
grep -q 'BACKUP_DIR}/syncleafimport' "${DEPLOY}" || fail "missing syncleafimport rollback restore"

# Must not auto-run or schedule leaf import.
if grep -E 'syncleafimport\.(service|timer)' "${DEPLOY}"; then
  fail "deploy must not install syncleafimport systemd units"
fi
if grep -E 'ExecStart=.*syncleafimport|systemctl .*(start|restart|enable).*syncleafimport' "${DEPLOY}"; then
  fail "deploy must not start/enable syncleafimport"
fi
# Deploy must not invoke the CLI during install.
if grep -E '[^[:alnum:]_]syncleafimport[[:space:]]+--' "${DEPLOY}"; then
  fail "deploy must not execute syncleafimport with flags"
fi

# Repo must not ship systemd units for syncleafimport.
if compgen -G "${ROOT}/deploy/systemd/syncleafimport.*" >/dev/null 2>&1; then
  fail "repo must not contain syncleafimport systemd unit files"
fi
if find "${ROOT}/deploy" -type f \( -name 'syncleafimport.service' -o -name 'syncleafimport.timer' \) 2>/dev/null | grep -q .; then
  fail "repo must not contain syncleafimport.service or syncleafimport.timer"
fi

# Worker guard must include leaf import binary.
grep -q 'worker_bins_running "${SYNC_BIN}" "${ENRICH_BIN}" "${MIGRATE_BIN}" "${LEAF_BIN}" "${LEAF_IMPORT_BIN}"' "${DEPLOY}" \
  || fail "worker_bins_running must include LEAF_IMPORT_BIN"

# cmd must exist in backend.
[[ -f "${ROOT}/backend/cmd/syncleafimport/main.go" ]] || fail "missing backend/cmd/syncleafimport/main.go"

printf 'ALL syncleafimport deploy tests passed\n'
