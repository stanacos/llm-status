# Changelog

## v0.3.0
- Hardened dashboard refresh scheduling to avoid overlapping fetches and queue a single follow-up refresh when needed.
- Added TTL caching + in-flight dedupe for expensive provider lookups (cost, version, OpenCode quota) with stale-on-error behavior.
- Fixed Codex cost timezone handling to prefer local day semantics and avoid forcing UTC when timezone cannot be resolved.
- Reworked Codex session log parsing to tolerate very large JSONL lines without aborting status extraction.
- Made OpenCode quota reset-date parsing more resilient to format variations and non-fatal on parse errors.
- Preserved warm-up error visibility across subsequent successful refreshes and bounded dashboard error history growth.
- Improved config fallback behavior when new config has an invalid provider entry.
- Fixed panel title border width calculations for wide/non-ASCII titles.
- Added regression tests for refresh dedupe, stale cache behavior, large-log parsing, timezone resolution, reset-date parsing fallback, and title width rendering.

## v0.2.2
- Fixed OpenCode Copilot quota auth flow to try direct auth token usage before token exchange.
- Added Copilot token exchange fallbacks across HTTP methods and user auth header formats to handle endpoint variations.
- Added regression tests for direct-token quota success and token-exchange fallback behavior.

## v0.2.1
- Fixed OpenCode auth token discovery to support additional auth.json token keys (including `access`).
- Fixed OpenCode cost parsing when `npx` emits non-JSON wrapper lines before JSON output.
- Fixed OpenCode version detection fallback to avoid blank header versions and support `opencode --version`.
- Updated OpenCode warm-up command to use `opencode --version` for broader CLI compatibility.

## v0.2.0
- Added OpenCode provider support.
- Added GitHub Copilot quota tracking with used/projected totals and monthly pace.
- Added provider-specific session and weekly panel labels/rows.
- Added OpenCode cost and token tracking via `@ccusage/opencode`.
- Added OpenCode warm-up support via `w`.
