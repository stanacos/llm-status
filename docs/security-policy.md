# Security and Data-Handling Policy

This project enforces strict redaction and least-exposure defaults.

## Allowed Public Identifiers

The following public identifiers may appear in tracked files:
- repository/module ownership references (`github.com/stanacos/*`)
- Homebrew tap ownership references (`stanacos/homebrew-tap`)
- GitHub noreply address references used in public commit metadata (`stanacos@users.noreply.github.com`)

All other personal identifiers must be treated as sensitive unless explicitly approved.

## Forbidden in Tracked Content

Do not commit:
- secrets or credentials (API keys, tokens, private keys, passwords)
- private personal identifiers (personal emails, phone numbers, SSNs)
- local machine identity data (absolute personal home paths, host/user identifiers)
- personal usage artifacts from local CLI auth/session stores

## Runtime Error Redaction Rules

UI-visible errors must not include:
- raw auth/token values
- raw HTTP response body excerpts from provider APIs
- raw command stderr/argument details
- absolute local filesystem paths

Detailed diagnostics are opt-in only via `LLM_STATUS_DEBUG_LOG=<path>` and must remain local.

## Compliance Enforcement

- `make compliance-tracked`: scans tracked files for PII/secrets.
- `make compliance-history`: scans git history diffs for PII/secrets.
- `make compliance`: runs both checks.

Allowlist management:
- file: `scripts/compliance/allowlist-public-ids.txt`
- only approved public identifiers may be allowlisted
- never allowlist private secrets or personal data

## History Rewrite Requirement

If sensitive history is found, use the rewrite/purge process in `docs/compliance-runbook.md` before release.
