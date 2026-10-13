#!/usr/bin/env bash
# Safe detection of production VPlayer worker binaries currently executing.
# Sourced by deploy-production.sh. Override PROC_ROOT for tests.
#
# Matches ONLY when /proc/<pid>/exe resolves to an exact absolute path listed
# by the caller. Does not print command lines or environment.

# Pure path match (testable without /proc). Canonicalizes both sides when possible.
exe_matches_worker_bins() {
  local exe="$1"
  shift
  local want want_res
  [[ -n "${exe}" ]] || return 1
  for want in "$@"; do
    [[ -n "${want}" ]] || continue
    if [[ "${exe}" == "${want}" ]]; then
      return 0
    fi
    want_res="$(readlink -f "${want}" 2>/dev/null || true)"
    if [[ -n "${want_res}" && "${exe}" == "${want_res}" ]]; then
      return 0
    fi
  done
  return 1
}

# Returns 0 if any listed absolute binary path is the executable of a live process.
# Usage: worker_bins_running /abs/path/syncincremental /abs/path/enrichpending
worker_bins_running() {
  local proc_root="${PROC_ROOT:-/proc}"
  local pid exe
  local -a wants=("$@")
  [[ "${#wants[@]}" -gt 0 ]] || return 1

  for pid in "${proc_root}"/[0-9]*; do
    [[ -e "${pid}/exe" || -L "${pid}/exe" ]] || continue
    exe="$(readlink -f "${pid}/exe" 2>/dev/null || true)"
    [[ -n "${exe}" ]] || continue
    if exe_matches_worker_bins "${exe}" "${wants[@]}"; then
      return 0
    fi
  done
  return 1
}
