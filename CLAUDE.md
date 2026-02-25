# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Summary

`llm-status` is a Go Bubble Tea TUI for monitoring usage across:
- Claude Code
- OpenAI Codex
- OpenCode

It shows session/weekly utilization, reset windows, daily and 30-day cost/token totals, provider-specific quota details, and provider CLI version.

## Core Commands

```bash
make build     # Build ./llm-status with version/commit/date ldflags
make run       # Run with build metadata ldflags
make test      # go test ./...
make vet       # go vet ./...
make fmt       # gofmt -w *.go
make clean     # remove ./llm-status binary
```

Version output:

```bash
./llm-status --version
./llm-status -v
```

## Code Layout

Single-package app (`package main`) at repo root.

- `main.go`: entrypoint and `--version`/`-v` handling.
- `model.go`: Bubble Tea state machine, key handling (`q`, `r`, `p`, `w`), refresh/tick orchestration, and rendering.
- `fetch.go`: provider fetch logic, command execution, HTTP calls, parsing, caching/dedupe helpers, warm-up commands.
- `types.go`: domain types, provider metadata, message types, JSON structs.
- `config.go`: persistent config (`~/.llm-status/config.json`), legacy migration from `~/.claude-status/config.json`.
- `components.go`, `theme.go`: rendering helpers and visual theme.

## Important Runtime Behavior (v0.3.0)

- Refresh interval is `60s`.
- Update loop prevents overlapping fetches; manual refresh while in-flight queues one follow-up refresh.
- Expensive data fetches are cached with TTL + in-flight dedupe:
  - Cost: `10m`
  - Version: `30m`
  - OpenCode quota: `5m`
- Stale-on-error is used for cached resources (with warning errors) to keep dashboard panels populated.
- Warm-up completion triggers refresh for the active provider; warm-up errors are preserved in the error list.

## Provider Data Sources

### Claude
- OAuth usage API (`~/.claude/.credentials.json`, macOS Keychain fallback)
- `npx ccusage@latest`
- `claude --version`

### Codex
- `~/.codex/sessions/*.jsonl` (`token_count` events)
- `npx @ccusage/codex@latest`
- `codex --version`

### OpenCode
- Copilot quota API via OpenCode auth (`~/.local/share/opencode/auth.json` or `$OPENCODE_DATA_DIR/auth.json`)
- `npx @ccusage/opencode@latest`
- `opencode version` (fallback `opencode --version`)

## Config Notes

- Current config includes:
  - `config_version` (additive, currently `1`)
  - `last_provider`
- Invalid provider values are treated as not-found to allow legacy fallback.
- Config writes are atomic (temp file + rename, restrictive file modes).

## Testing Guidance

- Use table-driven tests.
- Keep tests adjacent to source (`*_test.go`).
- Prefer mocked function vars over live external dependencies.
- Useful targets:
  - Refresh dedupe and warm-up flow behavior (`model_test.go`)
  - Log and date/time parsing (`fetch_test.go`, `fetch_opencode_test.go`)
  - Config fallback/migration behavior (`config_test.go`)

## Release Workflow

- Merge changes into `main` via PR.
- Create and push annotated tag `vX.Y.Z`.
- Tag triggers release workflow (`.github/workflows/release.yml`) that:
  - runs vet/tests
  - builds binaries
  - publishes GitHub Release assets
  - updates Homebrew tap (`stanacos/homebrew-tap`)

## Conventions

- Conventional commits (`feat:`, `fix:`, `chore:`).
- Keep UI concerns in `model.go`, data retrieval/parsing in `fetch.go`.
- Wrap errors with context (`fmt.Errorf("...: %w", err)`).
