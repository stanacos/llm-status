# llm-status

`llm-status` is a terminal dashboard for Claude Code, OpenAI Codex, and OpenCode usage. It shows provider-specific session/weekly utilization and reset data, daily/30-day cost and token totals, provider quota data, and provider CLI version.

## Install

### Homebrew
```bash
brew tap stanacos/homebrew-tap
brew install llm-status
```

### Go install
```bash
go install github.com/stanacos/llm-status@latest
```

### GitHub Releases
Prebuilt binaries are published on GitHub Releases (macOS/Linux, amd64/arm64):

https://github.com/stanacos/llm-status/releases

## Prerequisites

- Claude provider data: `claude` CLI installed/authenticated (`~/.claude/.credentials.json`).
- Codex provider data: `codex` CLI installed and session logs in `~/.codex/sessions/*.jsonl`.
- OpenCode provider data: `opencode` CLI installed/authenticated and auth at `~/.local/share/opencode/auth.json` (or `$OPENCODE_DATA_DIR/auth.json`).
- OpenCode quota data requires GitHub API access (`api.github.com` Copilot endpoints).
- Node.js with `npx` is required for full functionality (`ccusage`, `@ccusage/codex`, and `@ccusage/opencode` are run via `npx`).
- Homebrew installs Node.js automatically as a dependency for `llm-status`.
- If installed without Homebrew and `npx` is unavailable, the app still works for rate-limit/version data; cost panels remain unavailable (`N/A`).

## Usage

Run:

```bash
llm-status
```

Keybindings:

- `q`: quit
- `r`: refresh now
- `p`: open provider chooser
- `w`: warm up current provider (Claude/Codex/OpenCode) and refresh

Provider chooser controls: `↑/↓` (or `j/k`) and `Enter`.

## Version

```bash
llm-status --version
llm-status -v
```

Output format:

```text
llm-status <version> (commit: <commit>, built: <date>)
```

## Config Migration

If legacy config exists at `~/.claude-status/config.json`, it is loaded and best-effort migrated to `~/.llm-status/config.json`.
