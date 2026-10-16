#!/usr/bin/env bash
# Pure helpers for VPlayer MySQL backup naming, retention, and freshness.
# Sourced by vplayer-db-backup.sh and tests. No credentials; no Docker required.

# Completed backup filename: vplayer-mysql-YYYYMMDDTHHMMSSZ.sql.gz
VPLAYER_DB_BACKUP_RE='^vplayer-mysql-[0-9]{8}T[0-9]{6}Z\.sql\.gz$'
VPLAYER_DB_BACKUP_PARTIAL_SUFFIX='.partial'

is_recognized_db_backup_name() {
  local name="$1"
  [[ "${name}" =~ ${VPLAYER_DB_BACKUP_RE} ]]
}

is_partial_db_backup_name() {
  local name="$1"
  [[ "${name}" == *.sql.gz"${VPLAYER_DB_BACKUP_PARTIAL_SUFFIX}" ]] ||
    [[ "${name}" == *.partial ]]
}

# Newest completed backup basename in dir, or empty.
newest_completed_db_backup() {
  local dir="$1"
  local f newest="" newest_mtime=0 m
  [[ -d "${dir}" ]] || return 0
  for f in "${dir}"/vplayer-mysql-*.sql.gz; do
    [[ -f "${f}" ]] || continue
    local base
    base="$(basename "${f}")"
    is_recognized_db_backup_name "${base}" || continue
    m="$(stat -c '%Y' "${f}" 2>/dev/null || stat -f '%m' "${f}" 2>/dev/null || echo 0)"
    if (( m >= newest_mtime )); then
      newest_mtime="${m}"
      newest="${base}"
    fi
  done
  printf '%s' "${newest}"
}

# Age in seconds of named backup file; empty if missing.
db_backup_age_seconds() {
  local path="$1"
  local now m
  [[ -f "${path}" ]] || return 0
  now="$(date +%s)"
  m="$(stat -c '%Y' "${path}" 2>/dev/null || stat -f '%m' "${path}" 2>/dev/null || echo 0)"
  printf '%s' "$((now - m))"
}

# Freshness: ok | warn_missing | fail_stale
# warn_missing = no completed backup
# fail_stale = newest older than max_age_sec (default 36h)
db_backup_freshness_status() {
  local dir="$1"
  local max_age_sec="${2:-129600}" # 36h
  local name age
  name="$(newest_completed_db_backup "${dir}")"
  if [[ -z "${name}" ]]; then
    printf 'warn_missing'
    return 0
  fi
  age="$(db_backup_age_seconds "${dir}/${name}")"
  if [[ -z "${age}" ]]; then
    printf 'warn_missing'
    return 0
  fi
  if (( age > max_age_sec )); then
    printf 'fail_stale'
    return 0
  fi
  printf 'ok'
}

# Delete oldest recognized completed backups beyond keep_count.
# Never deletes partials via this path (caller removes own partials).
# Prints deleted basenames to stdout (one per line).
# Uses mtime-ascending order (oldest first). Portable: ls -1tr.
prune_db_backups() {
  local dir="$1"
  local keep_count="${2:-14}"
  local -a files=()
  local f base
  [[ -d "${dir}" ]] || return 0
  (( keep_count >= 1 )) || keep_count=14

  shopt -s nullglob
  local -a candidates=("${dir}"/vplayer-mysql-*.sql.gz)
  shopt -u nullglob
  if [[ "${#candidates[@]}" -eq 0 ]]; then
    return 0
  fi

  while IFS= read -r f; do
    [[ -n "${f}" && -f "${f}" ]] || continue
    base="$(basename "${f}")"
    is_recognized_db_backup_name "${base}" || continue
    files+=("${f}")
  done < <(ls -1tr "${candidates[@]}" 2>/dev/null || true)

  local count="${#files[@]}"
  local remove=$((count - keep_count))
  if (( remove <= 0 )); then
    return 0
  fi
  local i
  for ((i = 0; i < remove; i++)); do
    base="$(basename "${files[$i]}")"
    if is_recognized_db_backup_name "${base}"; then
      rm -f "${files[$i]}"
      printf '%s\n' "${base}"
    fi
  done
}

make_db_backup_basename() {
  # UTC timestamp
  printf 'vplayer-mysql-%s.sql.gz' "$(date -u +%Y%m%dT%H%M%SZ)"
}

# Byte size of a file, or empty on failure.
backup_partial_byte_size() {
  local path="$1"
  [[ -f "${path}" ]] || return 1
  stat -c '%s' "${path}" 2>/dev/null || stat -f '%z' "${path}" 2>/dev/null || return 1
}

# Validate a completed .partial before atomic rename.
# Requires: file exists, size >= min_size (default 64), and gzip -t succeeds.
# On success prints byte size to stdout and returns 0; otherwise returns non-zero.
validate_backup_partial_for_finalize() {
  local path="$1"
  local min_size="${2:-64}"
  local sz

  [[ -f "${path}" ]] || return 1
  sz="$(backup_partial_byte_size "${path}")" || return 1
  if ! [[ "${sz}" =~ ^[0-9]+$ ]] || (( sz < min_size )); then
    return 1
  fi
  gzip -t "${path}" 2>/dev/null || return 1
  printf '%s' "${sz}"
  return 0
}
