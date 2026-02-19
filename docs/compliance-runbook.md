# Compliance Runbook

This runbook covers repository compliance checks and the rewrite/purge workflow if sensitive content is found in git history.

## Scope

The compliance checks look for:
- likely credential material (API tokens, private keys, cloud access IDs)
- likely PII (email addresses, US SSN format)

Scans are enforced in CI and should be run locally before opening a PR.

## Local Checks

Run tracked-file scan:

```bash
make compliance-tracked
```

Run git-history scan:

```bash
make compliance-history
```

Run both:

```bash
make compliance
```

Scanner scripts:
- `scripts/compliance-scan-tracked.sh`
- `scripts/compliance-scan-history.sh`

## Allowlist Management

Allowlist file:
- `scripts/compliance/allowlist-public-ids.txt`

Rules:
- Use allowlist entries only for known public identifiers (for example org/user IDs).
- Never allowlist actual secrets or private personal data.
- Prefer literal entries; use regex only when needed with `re:<pattern>` lines.

Example:

```text
github.com/stanacos
re:^github\.com/stanacos/
```

Optional override:

```bash
COMPLIANCE_ALLOWLIST_FILE=/path/to/allowlist.txt make compliance
```

## Violation Handling

If a scan fails:
1. Identify whether the hit is real sensitive data or a false positive.
2. If sensitive data is real, rotate/revoke credentials immediately.
3. Fix tracked-file violations in the working tree and re-run `make compliance-tracked`.
4. For history violations, use the rewrite workflow below.

## History Rewrite + Purge Workflow

Use a clean branch and coordinate with maintainers before rewriting shared history.

1. Create a backup mirror clone:

```bash
git clone --mirror https://github.com/stanacos/llm-status.git llm-status-backup.git
```

2. Rewrite history with `git-filter-repo`.

Replace sensitive text patterns:

```bash
cat > /tmp/replace-rules.txt <<'EOF_RULES'
regex:sk-[A-Za-z0-9]{20,}==>REDACTED_OPENAI_KEY
regex:gh[pousr]_[A-Za-z0-9_]{20,}==>REDACTED_GITHUB_TOKEN
EOF_RULES

git filter-repo --force --replace-text /tmp/replace-rules.txt
```

Or remove a committed secret file entirely:

```bash
git filter-repo --force --invert-paths --path path/to/secret.file
```

3. Prune rewrite remnants from local refs and object cache:

```bash
git for-each-ref --format='delete %(refname)' refs/original | git update-ref --stdin
git reflog expire --expire=now --all
git gc --prune=now --aggressive
```

4. Re-run compliance checks:

```bash
make compliance
```

5. Force-push rewritten history and tags:

```bash
git push origin --force --all
git push origin --force --tags
```

6. Coordinate cleanup:
- notify collaborators to rebase/re-clone against the rewritten history
- remove sensitive data from release assets, uploaded artifacts, and external mirrors if present

## CI Enforcement

CI runs compliance checks in `.github/workflows/ci.yml` using full history checkout (`fetch-depth: 0`).

Expected behavior:
- any compliance violation fails the `compliance` job
- existing build/test job remains unchanged and continues to run independently
