# Changelog

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
