#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/compliance/common.sh
source "${SCRIPT_DIR}/compliance/common.sh"

allowlist_file="${COMPLIANCE_ALLOWLIST_FILE:-${DEFAULT_ALLOWLIST_FILE}}"

usage() {
  cat <<'USAGE'
Usage: scripts/compliance-scan-history.sh [--allowlist PATH]

Scans full git history diff content for security/PII signatures.
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

if [[ "$(git rev-parse --is-shallow-repository)" == "true" ]]; then
  echo "History scan requires a full git clone; detected shallow repository." >&2
  echo "Re-run after fetching full history (for CI use actions/checkout fetch-depth: 0)." >&2
  exit 2
fi

load_allowlist "$allowlist_file"
print_scan_header "Git history compliance scan"

history_snapshot="$(mktemp)"
metadata_snapshot="$(mktemp)"
trap 'rm -f "$history_snapshot" "$metadata_snapshot"' EXIT

declare -a scan_refs=()
while IFS= read -r ref_name; do
  if [[ -n "$ref_name" ]]; then
    scan_refs+=("$ref_name")
  fi
done < <(git for-each-ref --format='%(refname)' refs/heads refs/tags)

if (( ${#scan_refs[@]} == 0 )); then
  scan_refs=("HEAD")
fi

git log --no-color -p --pretty=format:'commit %H' "${scan_refs[@]}" > "$history_snapshot"
git log --format='%H:%an <%ae>' "${scan_refs[@]}" > "$metadata_snapshot"

violations=0

while IFS='|' read -r rule_id rule_desc rule_regex; do
  if [[ -z "$rule_id" ]]; then
    continue
  fi

  matches="$(awk -v re="$rule_regex" '
    /^commit [0-9a-f]{40}$/ {
      commit = $2
      next
    }
    /^\+\+\+ b\// {
      file = substr($0, 7)
      next
    }
    $0 == "+++ /dev/null" {
      file = "<deleted-file>"
      next
    }
    /^[+-][^+-]/ {
      content = substr($0, 2)
      if (content ~ re) {
        printf "%s:%s:%s\\n", commit, file, content
      }
    }
  ' "$history_snapshot")"

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

while IFS='|' read -r rule_id rule_desc rule_regex; do
  if [[ -z "$rule_id" ]]; then
    continue
  fi

  matches="$(grep -nE -- "$rule_regex" "$metadata_snapshot" || true)"
  if [[ -z "$matches" ]]; then
    continue
  fi

  filtered_matches="$(printf '%s\n' "$matches" | filter_allowlisted_lines)"
  if [[ -z "$filtered_matches" ]]; then
    continue
  fi

  violations=$((violations + 1))
  printf '\n[%s] %s (commit metadata)\n%s\n' "$rule_id" "$rule_desc" "$filtered_matches"
done < <(compliance_rules)

if (( violations > 0 )); then
  printf '\nGit history compliance scan failed: %d rule(s) with violations.\n' "$violations" >&2
  exit 1
fi

echo "Git history compliance scan passed."
