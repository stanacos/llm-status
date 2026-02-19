# llm-status

`llm-status` is a terminal dashboard for Claude Code and OpenAI Codex usage. It shows rate-limit utilization and reset times, daily/30-day cost and token totals, and provider CLI version.

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
- Node.js with `npx` is required for full functionality (`ccusage` and `@ccusage/codex` are run via `npx`).
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
