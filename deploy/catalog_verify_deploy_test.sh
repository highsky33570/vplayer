#!/usr/bin/env bash
# Validates deploy-production.sh fails the deploy when the catalog API is not HTTP 200.
# Does not contact production and does not run systemctl.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEPLOY="${ROOT}/deploy/deploy-production.sh"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

[[ -f "${DEPLOY}" ]] || fail "missing ${DEPLOY}"

grep -q 'curl -fsS --max-time 10 "${health_url}"' "${DEPLOY}" || fail "missing /health check"
grep -q 'homepage HTTP check failed' "${DEPLOY}" || fail "missing homepage check"
grep -q '/api/v1/videos?limit=1' "${DEPLOY}" || fail "missing catalog endpoint check"
grep -q 'catalog HTTP check failed' "${DEPLOY}" || fail "catalog non-200 must fail the deploy"

# The catalog check must die before success is declared.
catalog_line="$(grep -n 'catalog HTTP check failed' "${DEPLOY}" | head -n1 | cut -d: -f1)"
success_line="$(grep -n 'DEPLOYMENT SUCCEEDED' "${DEPLOY}" | head -n1 | cut -d: -f1)"
[[ -n "${catalog_line}" && -n "${success_line}" ]] || fail "could not locate catalog check or success line"
if (( catalog_line >= success_line )); then
  fail "catalog check must run before DEPLOYMENT SUCCEEDED"
fi

# die() exits non-zero, and the EXIT trap rolls back once install has started.
grep -q 'die "catalog HTTP check failed' "${DEPLOY}" || fail "catalog failure must call die"
grep -q 'rollback_from_backup' "${DEPLOY}" || fail "missing rollback"

echo "catalog deploy verification: ok"
