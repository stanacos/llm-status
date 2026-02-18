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
