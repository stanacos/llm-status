# PRD: llm-status Distribution & Release Infrastructure

**Author:** stanacos
**Date:** 2026-02-18
**Status:** Draft
**Version:** 1.0
**Ralphy Compatible:** Yes

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Problem Statement](#problem-statement)
3. [Goals & Success Metrics](#goals--success-metrics)
4. [User Stories](#user-stories)
5. [Functional Requirements](#functional-requirements)
6. [Non-Functional Requirements](#non-functional-requirements)
7. [Technical Considerations](#technical-considerations)
8. [Implementation Roadmap](#implementation-roadmap)
9. [Out of Scope](#out-of-scope)
10. [Open Questions & Risks](#open-questions--risks)
11. [Validation Checkpoints](#validation-checkpoints)
12. [Tasks](#tasks)

---

## Executive Summary

llm-status is a working TUI dashboard for monitoring LLM coding assistant usage (Claude Code, OpenAI Codex), but it can only be run from source via `go run .`. This PRD covers making llm-status installable via `go install`, Homebrew, and GitHub Release binaries, along with adding version reporting, runtime dependency checks, and automated CI/CD release infrastructure.

---

## Problem Statement

### Current Situation
llm-status is a fully functional Go TUI app that requires cloning the repository and running `go run .` or building a local binary. There is no install path for users who don't have Go or the source code. A 10MB binary is tracked in git with no `.gitignore`. The `go.mod` module path (`github.com/stana/llm-status`) does not match the GitHub repo URL (`github.com/stanacos/llm-status`), which prevents `go install` from working.

### User Impact
- **Who is affected:** Developers who want to monitor their LLM usage but don't have Go installed or don't want to clone the repo
- **How they're affected:** Cannot install or use the tool without Go toolchain knowledge; no standard install path (Homebrew, binary download)
- **Severity:** High -- blocks adoption entirely for non-Go users

### Business Impact
- **Cost of problem:** Zero adoption beyond the author; tool is effectively private
- **Opportunity cost:** Useful tool not reaching the community of Claude Code and Codex users
- **Strategic importance:** First public release establishes the project as a community tool

### Why Solve This Now?
The tool is feature-complete and stable. The only barrier to sharing it is the lack of distribution infrastructure.

---

## Goals & Success Metrics

### Goal 1: Installable via `go install`
- **Description:** Users with Go can install with a single command
- **Metric:** `go install github.com/stanacos/llm-status@latest` succeeds and produces a working binary
- **Baseline:** Not possible (module path mismatch)
- **Target:** Works on first try for any user with Go 1.21+
- **Timeframe:** v0.1.0 release
- **Measurement Method:** Manual test from a clean environment

### Goal 2: Installable via Homebrew
- **Description:** macOS and Linux users can install via `brew install stanacos/tap/llm-status`
- **Metric:** Homebrew formula installs correctly and `llm-status --version` reports the correct version
- **Baseline:** No Homebrew tap exists
- **Target:** Tap auto-updated on every tagged release
- **Timeframe:** v0.1.0 release
- **Measurement Method:** `brew install stanacos/tap/llm-status && llm-status --version`

### Goal 3: Automated release pipeline
- **Description:** Tagging a version triggers automated cross-platform builds, GitHub Release, and Homebrew tap update
- **Metric:** `git tag v0.1.0 && git push --tags` produces a complete GitHub Release with Linux/macOS binaries (amd64 + arm64) and updates the Homebrew formula
- **Baseline:** No CI/CD exists
- **Target:** Fully automated, zero manual steps after tagging
- **Timeframe:** v0.1.0 release
- **Measurement Method:** GitHub Actions workflow succeeds, release page has 4 binary archives + checksums

---

## User Stories

### Story 1: Go User Installs llm-status

**As a** developer with Go installed,
**I want to** run `go install github.com/stanacos/llm-status@latest`,
**So that I can** use llm-status without cloning the repo.

**Acceptance Criteria:**
- `go install` succeeds and places `llm-status` binary in `$GOPATH/bin`
- Running `llm-status` launches the TUI dashboard
- Running `llm-status --version` prints version, commit hash, and build date

**Dependencies:** REQ-001, REQ-003

---

### Story 2: Homebrew User Installs llm-status

**As a** macOS or Linux user,
**I want to** run `brew install stanacos/tap/llm-status`,
**So that I can** install llm-status without Go.

**Acceptance Criteria:**
- `brew tap stanacos/tap` succeeds
- `brew install llm-status` downloads and installs the correct binary for the user's platform
- `llm-status --version` shows the installed version
- `brew upgrade llm-status` picks up new releases

**Dependencies:** REQ-005, REQ-006, REQ-007

---

### Story 3: User Without Node.js Sees Clear Warning

**As a** user without Node.js installed,
**I want to** see a clear message about what I'm missing,
**So that I** understand why cost data is unavailable and how to fix it.

**Acceptance Criteria:**
- On startup, if `npx` is not in PATH, an error message appears in the TUI
- The warning includes instructions (e.g., "install Node.js from https://nodejs.org")
- The app still functions for rate-limit data (session/weekly panels)
- Cost panels show "N/A" gracefully

**Dependencies:** REQ-004

---

## Functional Requirements

### Must Have (P0) - Critical for Launch

#### REQ-001: Fix go.mod Module Path
**Description:** Change the module declaration in `go.mod` from `github.com/stana/llm-status` to `github.com/stanacos/llm-status` so that `go install` resolves correctly.

**Acceptance Criteria:**
- `go.mod` line 1 reads `module github.com/stanacos/llm-status`
- `go mod tidy` succeeds after the change
- `go build ./...` succeeds
- `go test ./...` passes

**Technical Specification:**
```
File: go.mod, line 1
Old: module github.com/stana/llm-status
New: module github.com/stanacos/llm-status
```

No Go source files import the module path (single-package `main`), so only `go.mod` needs changing.

**Dependencies:** None

---

#### REQ-002: Remove Tracked Binary and Add .gitignore
**Description:** The 10MB `llm-status` binary is tracked in git. Remove it from tracking and add a `.gitignore` to prevent re-committing it.

**Acceptance Criteria:**
- `.gitignore` exists at repo root with entries for `llm-status`, `dist/`, and `.DS_Store`
- `git ls-files llm-status` returns empty (binary untracked)
- The binary still exists locally for the developer after `git rm --cached`

**Dependencies:** None

---

#### REQ-003: Add --version Flag with Build-Time Injection
**Description:** Add `version`, `commit`, and `date` variables to `main.go` that are injected via `-ldflags` at build time. When the user runs `llm-status --version` or `llm-status -v`, print version info and exit.

**Acceptance Criteria:**
- `llm-status --version` prints: `llm-status v0.1.0 (commit: abc1234, built: 2026-02-18T...)`
- `llm-status -v` does the same
- When built without ldflags (e.g., `go run .`), version shows `dev`
- The hardcoded User-Agent `"llm-status/1.0"` in `fetch.go` line 539 is updated to use the injected `version` variable

**Technical Specification:**
```go
// main.go - add before main()
var (
    version = "dev"
    commit  = "none"
    date    = "unknown"
)

// Parse --version/-v before starting TUI
if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
    fmt.Printf("llm-status %s (commit: %s, built: %s)\n", version, commit, date)
    os.Exit(0)
}
```

**Dependencies:** None

---

#### REQ-004: Add npx Runtime Check with TUI Warning
**Description:** On startup, check if `npx` is available in PATH. If missing, append a warning to the dashboard errors so the user sees it in the TUI. The app continues to function -- only cost data (Today/Monthly panels) is affected.

**Acceptance Criteria:**
- If `npx` is not in PATH, an error appears in the TUI: `"npx not found in PATH (required for cost data) - install Node.js: https://nodejs.org"`
- The error appears via the existing `m.data.Errors` mechanism (displayed in dashboard error line)
- Session and Weekly panels still show rate-limit data from OAuth/logs
- If `npx` IS available, no warning appears

**Technical Specification:**
```go
// fetch.go - add function
func checkNpxAvailable() error {
    _, err := exec.LookPath("npx")
    if err != nil {
        return fmt.Errorf("npx not found in PATH (required for cost data) - install Node.js: https://nodejs.org")
    }
    return nil
}

// model.go - in newModel(), after config loading (~line 46)
if err := checkNpxAvailable(); err != nil {
    m.data.Errors = append(m.data.Errors, err.Error())
}
```

**Dependencies:** None

---

#### REQ-005: goreleaser Configuration
**Description:** Create `.goreleaser.yaml` that cross-compiles for Linux and macOS (amd64 + arm64), injects version via ldflags, and publishes to the Homebrew tap.

**Acceptance Criteria:**
- `.goreleaser.yaml` exists at repo root
- `goreleaser check` passes (valid config)
- `goreleaser build --snapshot --clean` produces 4 binaries (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64)
- Config includes ldflags for `main.version`, `main.commit`, `main.date`
- Config includes `brews` section targeting `stanacos/homebrew-tap`
- Archives use `tar.gz` format
- Checksums file is generated

**Technical Specification:**
```yaml
version: 2
project_name: llm-status

builds:
  - main: .
    binary: llm-status
    env: [CGO_ENABLED=0]
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.ShortCommit}}
      - -X main.date={{.Date}}

archives:
  - formats: [tar.gz]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: checksums.txt

brews:
  - name: llm-status
    repository:
      owner: stanacos
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    directory: Formula
    homepage: "https://github.com/stanacos/llm-status"
    description: "TUI dashboard for LLM coding assistant usage"
    dependencies:
      - name: node
        type: optional
    caveats: "llm-status requires Node.js (npx) for cost tracking. Install via: brew install node"
```

**Dependencies:** REQ-003 (version variables must exist)

---

#### REQ-006: GitHub Actions Release Workflow
**Description:** Create `.github/workflows/release.yml` that triggers on tag push (`v*`), runs tests, then runs goreleaser to build, release, and publish Homebrew formula.

**Acceptance Criteria:**
- Workflow triggers on `push: tags: ["v*"]`
- Runs `go test ./...` before releasing
- Uses `goreleaser/goreleaser-action@v6` with `version: "~> v2"`
- Passes `GITHUB_TOKEN` and `HOMEBREW_TAP_GITHUB_TOKEN` as env vars
- Creates a GitHub Release with binary archives and checksums

**Technical Specification:**
```yaml
name: Release
on:
  push:
    tags: ["v*"]
permissions:
  contents: write
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go test ./...
      - uses: goreleaser/goreleaser-action@v6
        with: { version: "~> v2", args: "release --clean" }
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}
```

**Dependencies:** REQ-005 (goreleaser config)

---

### Should Have (P1) - Important but Not Blocking

#### REQ-007: GitHub Actions CI Workflow
**Description:** Create `.github/workflows/ci.yml` that runs on push to main and PRs. Runs `go vet`, `go test`, and `go build` to catch broken builds early.

**Acceptance Criteria:**
- Workflow triggers on push to main and pull requests
- Runs `go vet ./...`, `go test ./...`, `go build ./...`
- Fails the PR check if any step fails

**Dependencies:** REQ-001 (module path must be correct)

---

#### REQ-008: Makefile for Local Development
**Description:** Create a `Makefile` with targets for build (with ldflags), run, test, vet, fmt, and clean. Provides a convenient local development workflow.

**Acceptance Criteria:**
- `make build` produces `llm-status` binary with version info
- `make run` runs with version info injected
- `make test` runs tests
- `make clean` removes the binary
- `./llm-status --version` after `make build` shows version, commit, and date

**Technical Specification:**
```makefile
VERSION ?= dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	go build -ldflags '$(LDFLAGS)' -o llm-status .

run:
	go run -ldflags '$(LDFLAGS)' .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w *.go

clean:
	rm -f llm-status
```

**Dependencies:** REQ-003 (version variables)

---

#### REQ-009: Update README.md with Install Instructions
**Description:** Rewrite README.md with installation instructions for all three distribution channels (go install, Homebrew, GitHub Releases), prerequisites (Node.js), and usage instructions.

**Acceptance Criteria:**
- README has Homebrew install command (`brew install stanacos/tap/llm-status`)
- README has go install command (`go install github.com/stanacos/llm-status@latest`)
- README links to GitHub Releases for binary downloads
- README documents Node.js as an optional prerequisite for cost data
- README lists keyboard shortcuts (q, p, r, j/k, arrow keys)

**Dependencies:** REQ-001 (correct module path)

---

#### REQ-010: Create Homebrew Tap Repository
**Description:** Create the `stanacos/homebrew-tap` GitHub repository and configure the `HOMEBREW_TAP_GITHUB_TOKEN` secret in the llm-status repo.

**Acceptance Criteria:**
- Public GitHub repo `stanacos/homebrew-tap` exists
- Has a `Formula/` directory (goreleaser will populate it)
- Has a README explaining it's a Homebrew tap
- `HOMEBREW_TAP_GITHUB_TOKEN` secret is configured in `stanacos/llm-status` repo settings with a PAT that has write access to the tap repo

**Dependencies:** None (external action, not code)

---

### Nice to Have (P2) - Future Enhancement

#### REQ-011: Update AGENTS.md with New Build Commands
**Description:** Add documentation for `make build`, `make run`, and `llm-status --version` to AGENTS.md so AI agents working on the codebase know about the build system.

**Acceptance Criteria:**
- AGENTS.md includes `make build`, `make run`, `make test` commands
- Documents `--version` flag existence

**Dependencies:** REQ-003, REQ-008

---

## Non-Functional Requirements

### Build & Binary

- **Binary size:** < 15MB per platform (current unstripped binary is ~10MB; `-s -w` ldflags should reduce this)
- **Build time:** < 60 seconds for all 4 platform targets via goreleaser
- **Static linking:** `CGO_ENABLED=0` for fully portable binaries (no libc dependency)

### CI/CD Pipeline

- **Release time:** < 5 minutes from tag push to GitHub Release published
- **Reliability:** Pipeline should not require manual intervention
- **Security:** Secrets (HOMEBREW_TAP_GITHUB_TOKEN) stored as GitHub encrypted secrets, never logged

### Compatibility

- **Go version:** Works with Go 1.21+ (for `go install`)
- **Platforms:** Linux amd64, Linux arm64, macOS amd64 (Intel), macOS arm64 (Apple Silicon)
- **Terminal:** Requires 56x20 minimum terminal size, color support, Unicode braille characters

---

## Technical Considerations

### System Architecture

**Current Architecture:**
Single-package Go application (`package main`) with 7 source files. All code in repo root. No internal packages, no cmd/ directory.

**Proposed Changes:**
No architectural changes to the Go code. Changes are limited to:
1. Build metadata injection (3 variables in `main.go`)
2. One new function in `fetch.go` (`checkNpxAvailable`)
3. One line addition in `model.go` (call npx check)
4. Infrastructure files (goreleaser, GitHub Actions, Makefile, .gitignore)

**Key Components:**
1. **goreleaser** -- Cross-compilation, archiving, checksums, Homebrew formula generation
2. **GitHub Actions** -- CI/CD automation (test on PR, release on tag)
3. **Homebrew tap** -- macOS/Linux package distribution via `stanacos/homebrew-tap` repo

### External Dependencies

**Runtime (user's machine):**
1. **Node.js/npx** (optional) -- Required for cost/token data via `npx ccusage@latest` and `npx @ccusage/codex@latest`. App functions without it but cost panels show N/A.
2. **claude CLI** (optional) -- For Claude version display
3. **codex CLI** (optional) -- For Codex version and session rate-limit data

**Build/Release (CI only):**
1. **goreleaser v2** -- Cross-compilation and release management
2. **GitHub Actions** -- CI/CD runner
3. **Personal Access Token** -- For Homebrew tap repo write access

### Testing Strategy

- **Existing tests:** `config_test.go` (6 table-driven tests) -- must continue to pass
- **Manual testing:** Build with `make build`, verify `--version` output, verify npx warning by temporarily renaming npx
- **Release testing:** Run `goreleaser build --snapshot --clean` locally before first real release

---

## Implementation Roadmap

### Phase 1: Repository Cleanup & Core Changes

**Goal:** Fix module path, remove tracked binary, add version flag and npx check

- Create `.gitignore` with entries for `llm-status`, `dist/`, and `.DS_Store` (REQ-002)
- Run `git rm --cached llm-status` to untrack the binary (REQ-002)
- Update `go.mod` module path to `github.com/stanacos/llm-status` (REQ-001)
- Run `go mod tidy` to regenerate `go.sum` (REQ-001)
- Add `version`, `commit`, `date` variables and `--version`/`-v` flag parsing to `main.go` (REQ-003)
- Update hardcoded User-Agent in `fetch.go` to use `version` variable (REQ-003)
- Add `checkNpxAvailable()` function to `fetch.go` (REQ-004)
- Call `checkNpxAvailable()` in `newModel()` in `model.go` (REQ-004)
- Verify `go vet ./...` and `go test ./...` pass (REQ-001)

**Validation Checkpoint:** `go build -o llm-status . && ./llm-status --version` prints `llm-status dev (commit: ..., built: ...)`

---

### Phase 2: Build Infrastructure

**Goal:** Add Makefile and goreleaser config, verify local builds

- Create `Makefile` with build, run, test, vet, fmt, clean targets including ldflags (REQ-008)
- Verify `make build && ./llm-status --version` shows version, commit, and date (REQ-008)
- Create `.goreleaser.yaml` with Linux+macOS builds, ldflags, archives, checksums, and brews section (REQ-005)
- Run `goreleaser check` to validate config syntax (REQ-005)
- Run `goreleaser build --snapshot --clean` to verify cross-compilation produces 4 binaries (REQ-005)

**Validation Checkpoint:** `ls dist/` shows 4 binary archives (linux_amd64, linux_arm64, darwin_amd64, darwin_arm64)

---

### Phase 3: CI/CD & Distribution

**Goal:** Set up GitHub Actions and Homebrew tap

- Create GitHub repo `stanacos/homebrew-tap` with a README and empty `Formula/` directory (REQ-010)
- Generate a Personal Access Token with write access to `stanacos/homebrew-tap` (REQ-010)
- Add `HOMEBREW_TAP_GITHUB_TOKEN` as a repository secret in `stanacos/llm-status` (REQ-010)
- Create `.github/workflows/release.yml` with tag-triggered goreleaser release job (REQ-006)
- Create `.github/workflows/ci.yml` with go vet, test, build on push/PR (REQ-007)

**Validation Checkpoint:** Push to main triggers CI workflow; all checks pass

---

### Phase 4: Documentation & Release

**Goal:** Update docs and cut v0.1.0

- Rewrite `README.md` with install instructions and prerequisites (REQ-009)
- Update `AGENTS.md` with new build commands (REQ-011)
- Tag `v0.1.0` and push tag to trigger release pipeline (REQ-006)
- Verify GitHub Release has 4 binary archives and checksums.txt (REQ-006)
- Verify Homebrew formula auto-published to `stanacos/homebrew-tap` (REQ-005)
- Test Homebrew install and `go install` from clean environments (REQ-010, REQ-001)

**Validation Checkpoint:** All three install methods work and `llm-status --version` shows `v0.1.0`

---

### Task Dependencies Visualization

```
Phase 1 (Core):
  .gitignore + git rm --cached ─┐
  Fix go.mod + go mod tidy ─────┤
  Add --version flag ───────────┤
  Add npx check ────────────────┘─→ go vet + go test pass

Phase 2 (Build):
  Phase 1 ─→ Makefile ─→ .goreleaser.yaml ─→ goreleaser check ─→ snapshot build

Phase 3 (CI/CD):
  Create homebrew-tap repo ─┐
  Generate + add PAT ───────┤
  Phase 2 ──────────────────┘─→ release.yml + ci.yml

Phase 4 (Release):
  Phase 3 ─→ README + AGENTS.md ─→ Tag v0.1.0 ─→ Verify all install methods

Critical Path: Fix go.mod → --version flag → Makefile → goreleaser → homebrew-tap → release.yml → tag v0.1.0
```

---

## Out of Scope

1. **Windows support**
   - Reason: TUI uses Unicode braille characters that may not render correctly in Windows terminals; low demand initially
   - Future: Can add Windows builds to goreleaser config once tested

2. **Docker image**
   - Reason: TUI app doesn't make sense in a container; users need a terminal
   - Future: Not planned

3. **AUR / Snap / Flatpak / other package managers**
   - Reason: Homebrew + go install covers the primary audience; adding more registries adds maintenance burden
   - Future: Community can contribute packages if demand exists

4. **Auto-update mechanism**
   - Reason: Homebrew and go install handle updates; no need for built-in updater
   - Future: Not planned

5. **Plugin system or extensibility**
   - Reason: Out of scope for distribution work; the app is feature-complete for now

---

## Open Questions & Risks

### Open Questions

#### Q1: Should the Homebrew formula declare Node.js as a required or optional dependency?
- **Current Status:** Planned as optional with a caveat message
- **Options:** (A) Optional dependency with caveat, (B) Required dependency
- **Owner:** stanacos
- **Impact:** Low -- app works without Node.js, just missing cost data

### Risks & Mitigation

| Risk | Likelihood | Impact | Severity | Mitigation | Contingency |
|------|------------|--------|----------|------------|-------------|
| HOMEBREW_TAP_GITHUB_TOKEN scope issues | Medium | High | **High** | Test PAT permissions before first release | Fall back to manual formula updates |
| Binary in git history inflates clone size | Low | Low | **Low** | `git rm --cached` prevents future tracking | BFG cleanup later if needed |
| go.mod rename breaks existing local checkouts | Low | Low | **Low** | No external consumers yet | Announce in release notes |
| goreleaser v2 config syntax changes | Low | Medium | **Medium** | Pin goreleaser action to `~> v2` | Check goreleaser docs for migration |

---

## Validation Checkpoints

### Checkpoint 1: End of Phase 1
**Criteria:**
- `go vet ./...` passes
- `go test ./...` passes
- `go build -o llm-status .` succeeds
- `./llm-status --version` prints version info
- `git status` shows binary is no longer tracked

**If Failed:** Fix Go compilation errors or test failures before proceeding

---

### Checkpoint 2: End of Phase 2
**Criteria:**
- `make build` produces binary with version info
- `goreleaser check` validates config
- `goreleaser build --snapshot --clean` produces 4 platform binaries

**If Failed:** Fix goreleaser config syntax or build issues

---

### Checkpoint 3: End of Phase 3
**Criteria:**
- `stanacos/homebrew-tap` repo exists and is public
- `HOMEBREW_TAP_GITHUB_TOKEN` secret is set in llm-status repo
- Push to main triggers CI workflow successfully

**If Failed:** Check GitHub Actions permissions and PAT scope

---

### Checkpoint 4: Post-Release (v0.1.0)
**Criteria:**
- GitHub Release exists with 4 archives + checksums
- `go install github.com/stanacos/llm-status@v0.1.0` works
- `brew install stanacos/tap/llm-status` works
- `llm-status --version` shows `v0.1.0` in all cases

**If Failed:** Check goreleaser logs, fix, and re-tag as v0.1.1

---

## Tasks

> **Ralphy Format**: All tasks below use `- [ ]` at column 1. Ralphy parses these with `grep '^\- \[ \]'`.
> Section headers provide context but are ignored by Ralphy's parser.

### Phase 1: Repository Cleanup & Core Changes

- [ ] Create `.gitignore` at repo root with entries: `llm-status`, `dist/`, `.DS_Store`
- [ ] Run `git rm --cached llm-status` to untrack the 10MB binary from git
- [ ] Update `go.mod` line 1 from `module github.com/stana/llm-status` to `module github.com/stanacos/llm-status`
- [ ] Run `go mod tidy` to regenerate `go.sum` with the new module path
- [ ] Add `version`, `commit`, `date` package-level variables to `main.go` with defaults `"dev"`, `"none"`, `"unknown"`
- [ ] Add `--version` and `-v` flag parsing to `main.go` before the TUI starts (check `os.Args[1]`)
- [ ] Update the hardcoded `"llm-status/1.0"` User-Agent string in `fetch.go` line 539 to use the `version` variable
- [ ] Add `checkNpxAvailable()` function to `fetch.go` that uses `exec.LookPath("npx")` and returns an error with install instructions
- [ ] Call `checkNpxAvailable()` in `newModel()` in `model.go` after the config loading block and append any error to `m.data.Errors`
- [ ] Verify `go vet ./...` passes after all changes
- [ ] Verify `go test ./...` passes after all changes

### Phase 2: Build Infrastructure

- [ ] Create `Makefile` with targets: build (with ldflags), run, test, vet, fmt, clean
- [ ] Verify `make build && ./llm-status --version` outputs version, commit hash, and build date
- [ ] Create `.goreleaser.yaml` with builds for linux/darwin amd64/arm64, ldflags, tar.gz archives, checksums, and brews section for `stanacos/homebrew-tap`
- [ ] Validate goreleaser config by running `goreleaser check`
- [ ] Test cross-compilation locally with `goreleaser build --snapshot --clean` and verify 4 binaries are produced

### Phase 3: CI/CD & Distribution

- [ ] Create public GitHub repository `stanacos/homebrew-tap` with a README and empty `Formula/` directory
- [ ] Generate a GitHub Personal Access Token with write access to `stanacos/homebrew-tap` repository
- [ ] Add `HOMEBREW_TAP_GITHUB_TOKEN` as an encrypted repository secret in `stanacos/llm-status` settings
- [ ] Create `.github/workflows/release.yml` that triggers on `v*` tags, runs tests, and executes goreleaser with both token env vars
- [ ] Create `.github/workflows/ci.yml` that runs `go vet`, `go test`, and `go build` on push to main and pull requests

### Phase 4: Documentation & Release

- [ ] Rewrite `README.md` with install instructions (Homebrew, go install, GitHub Releases), prerequisites (Node.js), and usage/keybindings
- [ ] Update `AGENTS.md` to document `make build`, `make run`, `make test`, and `--version` flag
- [ ] Commit all changes, tag `v0.1.0`, and push tag to trigger the release pipeline
- [ ] Verify GitHub Release page has 4 binary archives and a checksums.txt file
- [ ] Verify Homebrew formula was auto-published to `stanacos/homebrew-tap/Formula/llm-status.rb`
- [ ] Test `go install github.com/stanacos/llm-status@v0.1.0` from a clean Go environment
- [ ] Test `brew tap stanacos/tap && brew install llm-status && llm-status --version` works correctly

---

**End of PRD**

*This PRD is optimized for Ralphy's checkbox task format. All tasks use `- [ ]` at column 1 for automated parsing. Section headers provide context for the AI agent executing tasks.*
