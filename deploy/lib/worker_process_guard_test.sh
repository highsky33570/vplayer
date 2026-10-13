#!/usr/bin/env bash
# Unit-style checks for worker_process_guard.sh (no real deploy / no real workers).
# Path matching is tested portably. Full /proc enumeration is Linux-oriented.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=worker_process_guard.sh
source "${ROOT}/deploy/lib/worker_process_guard.sh"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf 'PASS: %s\n' "$*"; }

SYNC_BIN="/opt/vplayer/backend/bin/syncincremental"
ENRICH_BIN="/opt/vplayer/backend/bin/enrichpending"
OTHER_BIN="/usr/bin/sleep"
STAGE_BIN="/tmp/vplayer-deploy.XXXX/bin/syncincremental"

# 1) empty wants / empty exe
if exe_matches_worker_bins "" "${SYNC_BIN}"; then
  fail "empty exe must not match"
fi
pass "empty exe ignored"

# 2) unrelated process path
if exe_matches_worker_bins "${OTHER_BIN}" "${SYNC_BIN}" "${ENRICH_BIN}"; then
  fail "unrelated process must not match"
fi
pass "unrelated process ignored"

# 3) incremental production binary
if ! exe_matches_worker_bins "${SYNC_BIN}" "${SYNC_BIN}" "${ENRICH_BIN}"; then
  fail "syncincremental should match"
fi
pass "incremental binary active"

# 4) enrichment production binary
if ! exe_matches_worker_bins "${ENRICH_BIN}" "${SYNC_BIN}" "${ENRICH_BIN}"; then
  fail "enrichpending should match"
fi
pass "enrichment binary active"

# 5) staging/tmp path must not match production paths
if exe_matches_worker_bins "${STAGE_BIN}" "${SYNC_BIN}" "${ENRICH_BIN}"; then
  fail "staging/tmp binary must not match"
fi
pass "staging binary ignored"

# 6) worker_bins_running with empty fake PROC_ROOT should be inactive
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
export PROC_ROOT="${TMP}/proc"
mkdir -p "${PROC_ROOT}"
if worker_bins_running "${SYNC_BIN}" "${ENRICH_BIN}"; then
  fail "empty proc should be inactive"
fi
pass "no active workers (empty proc)"

printf 'ALL worker_process_guard tests passed\n'
