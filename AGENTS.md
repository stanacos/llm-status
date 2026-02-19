# Repository Guidelines

## Project Structure & Module Organization
This is a single-package Go Bubble Tea TUI (`package main`) with source files at repo root.

- `main.go`: app entrypoint.
- `model.go`: UI state machine, provider selection screen, refresh loop, key handling (`q`, `r`, `p`).
- `fetch.go`: provider-specific data fetchers (Claude and Codex), command execution, API/log parsing.
- `types.go`: shared domain types (`ProviderID`, `DashboardData`, message types).
- `config.go`: persistent user config (`~/.llm-status/config.json`) for last selected provider.
- `components.go` / `theme.go`: rendering helpers and style palette.

Keep new code in root files unless a package split is clearly justified.

## Build, Test, and Development Commands
- `make build`: build `./llm-status` with build metadata injected via ldflags.
- `make run`: run the app with the same ldflags metadata injection.
- `make test`: run unit tests.
- `make vet`: run static checks.
- `make compliance`: run tracked-file and git-history PII/security scans.
- `make fmt`: format all Go source files.
- `make clean`: remove the local `./llm-status` binary.
- `./llm-status --version` (or `./llm-status -v`): prints `llm-status <version> (commit: <commit>, built: <date>)`.
- Build-time ldflags variables:
  - `VERSION` (default `dev`) -> `main.version`
  - `COMMIT` (default `git rev-parse --short HEAD` or `none`) -> `main.commit`
  - `DATE` (default UTC timestamp `YYYY-MM-DDTHH:MM:SSZ`) -> `main.date`

## Coding Style & Naming Conventions
Use idiomatic Go and keep code `gofmt`-clean.

- Use `camelCase` for unexported identifiers and `PascalCase` for exported types.
- Keep provider dispatch centralized in `fetchAllDataForProvider`.
- Keep UI decisions in `model.go`; keep data retrieval/parsing in `fetch.go`.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`).

## Testing Guidelines
Use Go’s `testing` package with table-driven tests.

- Place tests next to code as `*_test.go`.
- Prioritize parser tests for:
  - Codex `token_count` session-log parsing.
  - `ccusage` / `@ccusage/codex` JSON decoding.
- Mock command execution where practical; avoid relying on live credentials in unit tests.
- Run `go test ./...` before opening a PR.

## Commit & Pull Request Guidelines
Repository history uses conventional prefixes (`feat:`, `chore:`). Continue that pattern.

- Commit format: `type: imperative summary`.
- PRs should include intent, key code paths changed, and validation steps run.
- Include screenshots/terminal captures for UI behavior changes (chooser screen, provider switching, panel format changes).

## Security & Configuration Tips
Do not commit secrets or personal usage artifacts.

- Claude credentials: `~/.claude/.credentials.json`.
- Codex usage/status source: `~/.codex/sessions/*.jsonl`.
- App config: `~/.llm-status/config.json`.
- External tools invoked: `claude --version`, `codex --version`, `npx ccusage@latest`, `npx @ccusage/codex@latest`.

## Compliance Checks
- Run `make compliance` before PRs that touch credentials, auth, telemetry, docs, or CI.
- Tracked-file scanner: `scripts/compliance-scan-tracked.sh`.
- Git-history scanner: `scripts/compliance-scan-history.sh`.
- Security policy: `docs/security-policy.md`.
- Allowlist approved public IDs in `scripts/compliance/allowlist-public-ids.txt` only (never secrets/PII).
- Rewrite/purge workflow is documented in `docs/compliance-runbook.md`.
- UI/runtime errors must stay redacted. Use `LLM_STATUS_DEBUG_LOG=<path>` only when detailed local diagnostics are needed.

## Recent Delivered Changes (2026-02-19)

### v0.1.1 (`8f8f86f`)
- Fixed first-start reset-time glitches where session reset could show stale/implausible values.
- Added timestamp normalization for Codex `resets_at` values (supports Unix seconds and milliseconds).
- Added plausibility guards for session/weekly reset windows.
- Changed Codex startup parsing to avoid fallback to stale older session files when newest file has `token_count` but uninitialized rate limits.
- Added regression tests in `fetch_test.go` for pending token_count behavior, fallback behavior, and timestamp/plausibility parsing.
- Updated README Homebrew tap command to `stanacos/homebrew-tap`.
- Released via tag `v0.1.1`; GitHub Release and Homebrew formula updated automatically by GoReleaser.

### v0.1.2 (`5a6e65f`)
- Added provider-specific manual warm-up key `w` in dashboard footer:
  - Claude provider: silent non-interactive warm-up via `claude -p`.
  - Codex provider: silent non-interactive warm-up via `codex exec`.
- Warm-up runs only for the currently selected provider and triggers an immediate refresh when complete.
- Added warm-up completion Bubble Tea message type (`warmupFinishedMsg`) and model handling.
- Removed footer `Next` refresh countdown text to reduce UI clutter.
- Updated README keybindings with `w`.
- Added tests in `model_test.go` and `fetch_test.go` for warm-up dispatch, update-loop behavior, footer rendering, and error handling.
- Delivered with PR #3 and released via tag `v0.1.2`; Homebrew formula updated and verified through `brew upgrade`.

## Release / Homebrew Runbook

Use this flow for future versions.

1. Create feature branch from `main`.
2. Implement changes and tests.
3. Run:
   - `make vet`
   - `make test`
   - `make build`
4. Commit using conventional format (`feat: ...`, `fix: ...`, etc.).
5. Push branch and open PR against `main`.
6. Wait for CI (`.github/workflows/ci.yml`) to pass.
7. Merge PR to `main`.
8. Pull latest `main` locally.
9. Create annotated version tag:
   - `git tag -a vX.Y.Z -m "<summary>"`
10. Push tag:
   - `git push origin vX.Y.Z`
11. Verify release workflow (`.github/workflows/release.yml`) succeeds.
12. Verify GitHub Release assets exist for all targets and `checksums.txt`.
13. Verify Homebrew tap formula (`stanacos/homebrew-tap`) is updated to `X.Y.Z`.
14. Verify end-user upgrade path:
   - `brew update`
   - `brew upgrade llm-status`
   - `llm-status --version`

Notes:
- Tag pushes (`v*`) are the release trigger.
- Homebrew publishing relies on `HOMEBREW_TAP_GITHUB_TOKEN`.
