#!/usr/bin/env bash
# Safe MySQL option-file helpers for Plan 8 (bash).
# Writes --defaults-extra-file content without putting the password on an
# external process argv. No logging of credential values.
#
# Escaping algorithm (MySQL option-file double-quoted values):
#   1. Reject any value containing LF (\\n) or CR (\\r) — fail closed.
#   2. Escape \\ -> \\\\ and " -> \\".
#   3. Wrap the result in double quotes so spaces, #, ;, =, $, and other
#      non-escaped bytes are treated as literals (not comments / separators).
# Ref: MySQL "Using Option Files" escape rules for quoted values.

# Return 0 if value has no CR/LF.
mysql_option_value_is_safe() {
  local v="$1"
  [[ "${v}" != *$'\n'* && "${v}" != *$'\r'* ]]
}

# Print a double-quoted, escaped option-file value to stdout.
# Does not print a trailing newline. Returns 1 if value is rejected.
mysql_option_quote_value() {
  local v="$1"
  mysql_option_value_is_safe "${v}" || return 1
  v="${v//\\/\\\\}"
  v="${v//\"/\\\"}"
  # Builtin printf only (no /usr/bin/printf argv exposure).
  printf '%s' "\"${v}\""
}

# Write a [client] defaults file with user + password (mode 600).
# Uses shell builtins + redirection only.
write_mysql_client_defaults_file() {
  local path="$1"
  local user="$2"
  local password="$3"
  local user_q password_q
  local xtrace_was=0
  local rc=0

  # Defeat inherited `set -x` so credentials never appear in xtrace logs.
  case $- in *x*) xtrace_was=1; set +x ;; esac

  if ! mysql_option_value_is_safe "${user}" || ! mysql_option_value_is_safe "${password}"; then
    rc=1
  elif ! user_q="$(mysql_option_quote_value "${user}")"; then
    rc=1
  elif ! password_q="$(mysql_option_quote_value "${password}")"; then
    rc=1
  else
    umask 077
    if ! : >"${path}"; then
      rc=1
    elif ! chmod 600 "${path}"; then
      rc=1
    else
      if {
        printf '%s\n' '[client]'
        printf '%s\n' "user=${user_q}"
        printf '%s\n' "password=${password_q}"
      } >"${path}" && chmod 600 "${path}"; then
        rc=0
      else
        rc=1
        rm -f "${path}" 2>/dev/null || true
      fi
    fi
  fi

  [[ "${xtrace_was}" -eq 1 ]] && set -x
  return "${rc}"
}
