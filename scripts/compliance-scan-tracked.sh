#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/compliance/common.sh
source "${SCRIPT_DIR}/compliance/common.sh"

allowlist_file="${COMPLIANCE_ALLOWLIST_FILE:-${DEFAULT_ALLOWLIST_FILE}}"

usage() {
  cat <<'USAGE'
Usage: scripts/compliance-scan-tracked.sh [--allowlist PATH]

Scans tracked repository files for security/PII signatures.
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --allowlist)
      shift
      if [[ $# -eq 0 ]]; then
        echo "Missing value for --allowlist" >&2
        exit 2
      fi
      allowlist_file="$1"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
  shift
done

load_allowlist "$allowlist_file"
print_scan_header "Tracked file compliance scan"

violations=0

while IFS='|' read -r rule_id rule_desc rule_regex; do
  if [[ -z "$rule_id" ]]; then
    continue
  fi

  matches="$(git grep -nI -E -e "$rule_regex" -- . || true)"
  if [[ -z "$matches" ]]; then
    continue
  fi

  filtered_matches="$(printf '%s\n' "$matches" | filter_allowlisted_lines)"
  if [[ -z "$filtered_matches" ]]; then
    continue
  fi

  violations=$((violations + 1))
  printf '\n[%s] %s\n%s\n' "$rule_id" "$rule_desc" "$filtered_matches"
done < <(compliance_rules)

if (( violations > 0 )); then
  printf '\nTracked file compliance scan failed: %d rule(s) with violations.\n' "$violations" >&2
  exit 1
fi

echo "Tracked file compliance scan passed."
