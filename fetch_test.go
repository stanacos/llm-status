package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseLatestTokenCountFromFile(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name  string
		lines []string
		check func(*testing.T, *codexStatusSnapshot, error)
	}{
		{
			name: "returns pending when token_count exists without usable limits",
			lines: []string{
				`{"timestamp":"2026-02-19T10:00:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":null,"secondary":null}}}`,
			},
			check: func(t *testing.T, _ *codexStatusSnapshot, err error) {
				t.Helper()
				if !errors.Is(err, errTokenCountPending) {
					t.Fatalf("expected errTokenCountPending, got %v", err)
				}
			},
		},
		{
			name: "parses latest valid token_count snapshot",
			lines: []string{
				validTokenCountLine("2026-02-19T10:00:00Z", now.Add(2*time.Hour).Unix(), now.Add(4*24*time.Hour).Unix(), 10, 30),
				validTokenCountLine("2026-02-19T10:01:00Z", now.Add(3*time.Hour).Unix(), now.Add(5*24*time.Hour).Unix(), 15, 35),
			},
			check: func(t *testing.T, status *codexStatusSnapshot, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if status == nil {
					t.Fatal("expected status, got nil")
				}
				if !status.hasSession || !status.hasWeekly {
					t.Fatalf("expected both session and weekly data, got session=%v weekly=%v", status.hasSession, status.hasWeekly)
				}
				if got, want := status.sessionUtil, 15.0; got != want {
					t.Fatalf("sessionUtil: got %v want %v", got, want)
				}
				if got, want := status.weeklyUtil, 35.0; got != want {
					t.Fatalf("weeklyUtil: got %v want %v", got, want)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeJSONL(t, dir, "session.jsonl", tc.lines...)
			status, err := parseLatestTokenCountFromFile(path)
			tc.check(t, status, err)
		})
	}
}

func TestParseLatestTokenCountFromFileSkipsVeryLongLine(t *testing.T) {
	now := time.Now().UTC()
	hugeLine := fmt.Sprintf(
		`{"timestamp":"2026-02-19T09:59:00Z","type":"event_msg","payload":{"type":"assistant_message","message":"%s"}}`,
		strings.Repeat("x", 17*1024*1024),
	)
	validLine := validTokenCountLine(
		"2026-02-19T10:00:00Z",
		now.Add(2*time.Hour).Unix(),
		now.Add(4*24*time.Hour).Unix(),
		22,
		44,
	)

	dir := t.TempDir()
	path := writeJSONL(t, dir, "session.jsonl", hugeLine, validLine)

	status, err := parseLatestTokenCountFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status == nil {
		t.Fatal("expected status, got nil")
	}
	if !status.hasSession || !status.hasWeekly {
		t.Fatalf("expected session+weekly data, got session=%v weekly=%v", status.hasSession, status.hasWeekly)
	}
}

func TestSelectCodexStatusFromFiles(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		name  string
		files map[string][]string
		check func(*testing.T, *codexStatusSnapshot, error)
	}{
		{
			name: "does not fall back to older stale file when newest is pending",
			files: map[string][]string{
				"a-old.jsonl": {
					validTokenCountLine("2026-02-18T10:00:00Z", now.Add(2*time.Hour).Unix(), now.Add(4*24*time.Hour).Unix(), 20, 40),
				},
				"z-new.jsonl": {
					`{"timestamp":"2026-02-19T10:00:00Z","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":null,"secondary":null}}}`,
				},
			},
			check: func(t *testing.T, status *codexStatusSnapshot, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if status == nil {
					t.Fatal("expected status, got nil")
				}
				if status.hasSession || status.hasWeekly {
					t.Fatalf("expected no data, got session=%v weekly=%v", status.hasSession, status.hasWeekly)
				}
			},
		},
		{
			name: "falls back when newest file has no token_count records",
			files: map[string][]string{
				"a-old.jsonl": {
					validTokenCountLine("2026-02-18T10:00:00Z", now.Add(2*time.Hour).Unix(), now.Add(4*24*time.Hour).Unix(), 20, 40),
				},
				"z-new.jsonl": {
					`{"timestamp":"2026-02-19T10:00:00Z","type":"event_msg","payload":{"type":"assistant_message"}}`,
				},
			},
			check: func(t *testing.T, status *codexStatusSnapshot, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if status == nil {
					t.Fatal("expected status, got nil")
				}
				if !status.hasSession || !status.hasWeekly {
					t.Fatalf("expected fallback data, got session=%v weekly=%v", status.hasSession, status.hasWeekly)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			paths := make([]string, 0, len(tc.files))
			for name, lines := range tc.files {
				paths = append(paths, writeJSONL(t, dir, name, lines...))
			}
			status, err := selectCodexStatusFromFiles(paths)
			tc.check(t, status, err)
		})
	}
}

func TestParseResetTimestamp(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name     string
		raw      int64
		wantUnix int64
		wantOK   bool
	}{
		{name: "seconds", raw: now.Unix(), wantUnix: now.Unix(), wantOK: true},
		{name: "milliseconds", raw: now.UnixMilli(), wantUnix: now.Unix(), wantOK: true},
		{name: "invalid", raw: 0, wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseResetTimestamp(tc.raw)
			if ok != tc.wantOK {
				t.Fatalf("ok: got %v want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if got.Unix() != tc.wantUnix {
				t.Fatalf("unix: got %d want %d", got.Unix(), tc.wantUnix)
			}
		})
	}
}

func TestIsPlausibleResetTime(t *testing.T) {
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)

	if !isPlausibleResetTime(now, now.Add(2*time.Hour), sessionWindow) {
		t.Fatal("expected +2h session reset to be plausible")
	}
	if isPlausibleResetTime(now, now.Add(-10*time.Minute), sessionWindow) {
		t.Fatal("expected stale reset to be implausible")
	}
	if isPlausibleResetTime(now, now.Add(24*time.Hour), sessionWindow) {
		t.Fatal("expected far-future session reset to be implausible")
	}
}

func TestLocalTimezoneAt(t *testing.T) {
	tests := []struct {
		name     string
		now      time.Time
		envTZ    string
		inferred string
		want     string
	}{
		{
			name:     "prefers TZ environment",
			now:      time.Date(2026, 2, 19, 12, 0, 0, 0, time.FixedZone("Local", -8*3600)),
			envTZ:    "America/New_York",
			inferred: "America/Los_Angeles",
			want:     "America/New_York",
		},
		{
			name:     "uses non-local location name",
			now:      time.Date(2026, 2, 19, 12, 0, 0, 0, time.FixedZone("America/Chicago", -6*3600)),
			envTZ:    "",
			inferred: "America/Los_Angeles",
			want:     "America/Chicago",
		},
		{
			name:     "uses inferred timezone for Local",
			now:      time.Date(2026, 2, 19, 12, 0, 0, 0, time.FixedZone("Local", -8*3600)),
			envTZ:    "",
			inferred: "America/Los_Angeles",
			want:     "America/Los_Angeles",
		},
		{
			name:     "returns empty when no TZ can be inferred",
			now:      time.Date(2026, 2, 19, 12, 0, 0, 0, time.FixedZone("Local", -8*3600)),
			envTZ:    "",
			inferred: "",
			want:     "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := localTimezoneAt(tc.now, tc.envTZ, tc.inferred)
			if got != tc.want {
				t.Fatalf("timezone: got %q want %q", got, tc.want)
			}
		})
	}
}

func TestGetCachedCostUsesStaleDataOnRefreshError(t *testing.T) {
	originalNow := nowFunc
	originalCaches := resourceCaches
	defer func() {
		nowFunc = originalNow
		resourceCaches = originalCaches
	}()

	resetResourceCaches()
	current := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	nowFunc = func() time.Time { return current }

	calls := 0
	fetch := func() (*costResult, error) {
		calls++
		if calls == 1 {
			return &costResult{
				dailyCost:      2.5,
				dailyTokens:    2000,
				hasDailyData:   true,
				monthlyCost:    12.0,
				monthlyTokens:  9000,
				hasMonthlyData: true,
			}, nil
		}
		return nil, errors.New("transient failure")
	}

	first, stale, err := getCachedCost(ProviderClaude, fetch)
	if err != nil || stale {
		t.Fatalf("first fetch unexpected stale/err: stale=%v err=%v", stale, err)
	}
	if first == nil || !first.hasDailyData {
		t.Fatalf("expected cached cost data, got %+v", first)
	}
	if calls != 1 {
		t.Fatalf("calls after first fetch: got %d want 1", calls)
	}

	current = current.Add(2 * time.Minute)
	second, stale, err := getCachedCost(ProviderClaude, fetch)
	if err != nil || stale {
		t.Fatalf("expected in-ttl cache hit, got stale=%v err=%v", stale, err)
	}
	if second == nil || second.dailyCost != first.dailyCost {
		t.Fatalf("unexpected cached second result: %+v", second)
	}
	if calls != 1 {
		t.Fatalf("calls after ttl-hit: got %d want 1", calls)
	}

	current = current.Add(costCacheTTL + time.Second)
	third, stale, err := getCachedCost(ProviderClaude, fetch)
	if err == nil || !stale {
		t.Fatalf("expected stale result with refresh error, got stale=%v err=%v", stale, err)
	}
	if third == nil || third.dailyCost != first.dailyCost {
		t.Fatalf("expected stale cached data, got %+v", third)
	}
	if calls != 2 {
		t.Fatalf("calls after stale refresh failure: got %d want 2", calls)
	}

	current = current.Add(30 * time.Second)
	_, stale, err = getCachedCost(ProviderClaude, fetch)
	if err != nil || stale {
		t.Fatalf("expected failure-retry TTL cache hit, got stale=%v err=%v", stale, err)
	}
	if calls != 2 {
		t.Fatalf("calls after failure retry window hit: got %d want 2", calls)
	}
}

func TestGetCachedVersionDedupesInFlightFetches(t *testing.T) {
	originalNow := nowFunc
	originalCaches := resourceCaches
	defer func() {
		nowFunc = originalNow
		resourceCaches = originalCaches
	}()

	resetResourceCaches()
	nowFunc = func() time.Time {
		return time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)
	}

	var mu sync.Mutex
	calls := 0
	fetch := func() (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		return "1.2.3", nil
	}

	var wg sync.WaitGroup
	results := make(chan string, 2)
	errs := make(chan error, 2)
	stales := make(chan bool, 2)

	run := func() {
		defer wg.Done()
		version, stale, err := getCachedVersion(ProviderCodex, fetch)
		results <- version
		stales <- stale
		errs <- err
	}

	wg.Add(2)
	go run()
	go run()
	wg.Wait()
	close(results)
	close(stales)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	for stale := range stales {
		if stale {
			t.Fatal("did not expect stale result")
		}
	}
	for version := range results {
		if version != "1.2.3" {
			t.Fatalf("unexpected version: %q", version)
		}
	}
	if calls != 1 {
		t.Fatalf("expected a single in-flight fetch call, got %d", calls)
	}
}

func writeJSONL(t *testing.T, dir string, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func validTokenCountLine(ts string, sessionReset int64, weeklyReset int64, sessionUtil float64, weeklyUtil float64) string {
	return fmt.Sprintf(
		`{"timestamp":"%s","type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":%.1f,"window_minutes":300,"resets_at":%d},"secondary":{"used_percent":%.1f,"window_minutes":10080,"resets_at":%d}}}}`,
		ts,
		sessionUtil,
		sessionReset,
		weeklyUtil,
		weeklyReset,
	)
}

func TestWarmUpProviderDispatch(t *testing.T) {
	originalRunner := warmUpCommandRunner
	defer func() {
		warmUpCommandRunner = originalRunner
	}()

	type commandCall struct {
		name string
		args []string
	}

	var calls []commandCall
	warmUpCommandRunner = func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, commandCall{name: name, args: append([]string(nil), args...)})
		return []byte("ok"), nil
	}

	if err := warmUpProvider(ProviderClaude); err != nil {
		t.Fatalf("warmUpProvider(ProviderClaude) error: %v", err)
	}
	if err := warmUpProvider(ProviderCodex); err != nil {
		t.Fatalf("warmUpProvider(ProviderCodex) error: %v", err)
	}
	if err := warmUpProvider(ProviderOpenCode); err != nil {
		t.Fatalf("warmUpProvider(ProviderOpenCode) error: %v", err)
	}

	if len(calls) != 3 {
		t.Fatalf("expected 3 warm-up calls, got %d", len(calls))
	}
	if calls[0].name != "claude" {
		t.Fatalf("expected first command claude, got %q", calls[0].name)
	}
	if got, want := strings.Join(calls[0].args, "\x00"), strings.Join([]string{"-p", claudeWarmUpPrompt}, "\x00"); got != want {
		t.Fatalf("unexpected claude args: got %q want %q", calls[0].args, []string{"-p", claudeWarmUpPrompt})
	}
	if calls[1].name != "codex" {
		t.Fatalf("expected second command codex, got %q", calls[1].name)
	}
	if got, want := strings.Join(calls[1].args, "\x00"), strings.Join([]string{"exec", "--skip-git-repo-check", codexWarmUpPrompt}, "\x00"); got != want {
		t.Fatalf(
			"unexpected codex args: got %q want %q",
			calls[1].args,
			[]string{"exec", "--skip-git-repo-check", codexWarmUpPrompt},
		)
	}
	if calls[2].name != "opencode" {
		t.Fatalf("expected third command opencode, got %q", calls[2].name)
	}
	if got, want := strings.Join(calls[2].args, "\x00"), strings.Join([]string{"--version"}, "\x00"); got != want {
		t.Fatalf("unexpected opencode args: got %q want %q", calls[2].args, []string{"--version"})
	}
}

func TestWarmUpProviderErrors(t *testing.T) {
	t.Run("unknown provider", func(t *testing.T) {
		err := warmUpProvider(ProviderID("unknown"))
		if err == nil {
			t.Fatal("expected error for unknown provider")
		}
		if !strings.Contains(err.Error(), `unknown provider "unknown"`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("codex command failure is wrapped", func(t *testing.T) {
		originalRunner := warmUpCommandRunner
		defer func() {
			warmUpCommandRunner = originalRunner
		}()

		warmUpCommandRunner = func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "codex" {
				return nil, errors.New("boom")
			}
			return []byte("ok"), nil
		}

		err := warmUpProvider(ProviderCodex)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "codex warm-up failed") {
			t.Fatalf("expected wrapped codex error, got %v", err)
		}
	})
}

func TestSanitizeUIErrorMessageRedactsSensitiveValues(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", homeDir)

	raw := `copilot user API returned 401: {"authorization":"Bearer very-secret-token-123","token":"abc1234567890","path":"` +
		filepath.Join(homeDir, ".codex", "sessions", "session.jsonl") +
		`"}; token=query-secret-123`

	got := sanitizeUIErrorMessage(raw)
	if !strings.Contains(got, "returned 401") {
		t.Fatalf("expected status code to remain visible, got %q", got)
	}
	if strings.Contains(got, "returned 401:") {
		t.Fatalf("expected http body detail to be stripped, got %q", got)
	}

	for _, leaked := range []string{
		"very-secret-token-123",
		"abc1234567890",
		"query-secret-123",
		homeDir,
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("sanitized message leaked %q: %q", leaked, got)
		}
	}
}

func TestAppendSanitizedErrorFromErrRedactsAbsolutePaths(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", homeDir)

	var errs []string
	path := filepath.Join(homeDir, ".llm-status", "config.json")
	appendSanitizedErrorFromErr(&errs, "config: ", fmt.Errorf("open %s: permission denied", path))

	if len(errs) != 1 {
		t.Fatalf("expected 1 error entry, got %d", len(errs))
	}
	if strings.Contains(errs[0], homeDir) {
		t.Fatalf("expected redacted home path, got %q", errs[0])
	}
	if !strings.Contains(errs[0], "~/.llm-status/config.json") {
		t.Fatalf("expected redacted home-relative path, got %q", errs[0])
	}
}

func TestRunCommandOmitsArgsAndStderrDetails(t *testing.T) {
	t.Setenv(debugLogEnvVar, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	secretPath := filepath.Join(t.TempDir(), "secret-token-123")
	_, err := runCommand(ctx, "cat", secretPath)
	if err == nil {
		t.Fatal("expected command error")
	}

	got := err.Error()
	if !strings.Contains(got, "cat command failed") {
		t.Fatalf("expected wrapped command name, got %q", got)
	}
	for _, leaked := range []string{
		secretPath,
		"secret-token-123",
		"No such file",
	} {
		if strings.Contains(got, leaked) {
			t.Fatalf("command error leaked %q: %q", leaked, got)
		}
	}
}

func TestRunCommandWritesDebugLogWhenEnabled(t *testing.T) {
	debugLogPath := filepath.Join(t.TempDir(), "debug", "llm-status.log")
	t.Setenv(debugLogEnvVar, debugLogPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	secretPath := filepath.Join(t.TempDir(), "secret-token-xyz")
	_, err := runCommand(ctx, "cat", secretPath)
	if err == nil {
		t.Fatal("expected command error")
	}

	logData, err := os.ReadFile(debugLogPath)
	if err != nil {
		t.Fatalf("expected debug log file, got error: %v", err)
	}
	logText := string(logData)
	if !strings.Contains(logText, `command failed name="cat"`) {
		t.Fatalf("expected command debug entry, got %q", logText)
	}
	if !strings.Contains(logText, secretPath) {
		t.Fatalf("expected debug log to include command detail path %q, got %q", secretPath, logText)
	}
}
