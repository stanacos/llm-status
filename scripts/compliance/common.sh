#!/usr/bin/env bash
set -euo pipefail

COMPLIANCE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_ALLOWLIST_FILE="${COMPLIANCE_ROOT}/allowlist-public-ids.txt"

declare -a ALLOWLIST_LITERALS=()
declare -a ALLOWLIST_REGEXES=()

trim_spaces() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

load_allowlist() {
  local allowlist_file="${1:-${DEFAULT_ALLOWLIST_FILE}}"
  if [[ -z "$allowlist_file" ]]; then
    return
  fi

  if [[ ! -f "$allowlist_file" ]]; then
    if [[ "$allowlist_file" == "$DEFAULT_ALLOWLIST_FILE" ]]; then
      return
    fi

    echo "Allowlist file not found: $allowlist_file" >&2
    exit 2
  fi

  while IFS= read -r raw_line || [[ -n "$raw_line" ]]; do
    raw_line="${raw_line%%#*}"
    raw_line="$(trim_spaces "$raw_line")"
    if [[ -z "$raw_line" ]]; then
      continue
    fi

    if [[ "$raw_line" == re:* ]]; then
      ALLOWLIST_REGEXES+=("${raw_line#re:}")
      continue
    fi

    ALLOWLIST_LITERALS+=("$raw_line")
  done < "$allowlist_file"
}

is_allowlisted_line() {
  local line="$1"
  local literal
  local regex

  for literal in "${ALLOWLIST_LITERALS[@]}"; do
    if [[ -n "$literal" && "$line" == *"$literal"* ]]; then
      return 0
    fi
  done

  for regex in "${ALLOWLIST_REGEXES[@]}"; do
    if [[ -n "$regex" && "$line" =~ $regex ]]; then
      return 0
    fi
  done

  return 1
}

filter_allowlisted_lines() {
  local line

  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ -z "$line" ]]; then
      continue
    fi

    if is_allowlisted_line "$line"; then
      continue
    fi

    printf '%s\n' "$line"
  done
}

compliance_rules() {
  cat <<'RULES'
aws_access_key_id|AWS access key ID|AKIA[0-9A-Z]{16}
aws_temporary_access_key_id|AWS temporary access key ID|ASIA[0-9A-Z]{16}
github_token|GitHub token|gh[pousr]_[A-Za-z0-9_]{20,}
slack_token|Slack token|xox[baprs]-[A-Za-z0-9-]{10,}
anthropic_api_key|Anthropic API key|sk-ant-[A-Za-z0-9_-]{20,}
openai_api_key|OpenAI API key|sk-[A-Za-z0-9]{20,}
private_key_block|Private key material|-----BEGIN[[:space:]]+[A-Z ]*PRIVATE KEY-----
email_address|Email address (PII)|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+[.][A-Za-z]{2,}
us_ssn|US SSN (PII)|(^|[^0-9])[0-9]{3}-[0-9]{2}-[0-9]{4}([^0-9]|$)
RULES
}

print_scan_header() {
  local label="$1"
  echo "==> ${label}"
}
