package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadOpenCodeAuth(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, homeDir string)
		wantToken string
		wantErr   string
	}{
		{
			name: "valid auth file",
			setup: func(t *testing.T, homeDir string) {
				t.Helper()
				writeOpenCodeAuthFixture(t, filepath.Join(homeDir, ".local", "share", "opencode", "auth.json"), `{"github-copilot":{"oauth_token":"token-123"}}`)
			},
			wantToken: "token-123",
		},
		{
			name: "valid auth file with access key",
			setup: func(t *testing.T, homeDir string) {
				t.Helper()
				writeOpenCodeAuthFixture(
					t,
					filepath.Join(homeDir, ".local", "share", "opencode", "auth.json"),
					`{"github-copilot":{"access":"token-abc","refresh":"r","expires":"2026-03-01T00:00:00Z","type":"bearer"}}`,
				)
			},
			wantToken: "token-abc",
		},
		{
			name: "missing file",
			setup: func(_ *testing.T, _ string) {
			},
			wantErr: "read opencode auth file",
		},
		{
			name: "missing github-copilot token",
			setup: func(t *testing.T, homeDir string) {
				t.Helper()
				writeOpenCodeAuthFixture(t, filepath.Join(homeDir, ".local", "share", "opencode", "auth.json"), `{}`)
			},
			wantErr: "missing github-copilot oauth token",
		},
		{
			name: "malformed json",
			setup: func(t *testing.T, homeDir string) {
				t.Helper()
				writeOpenCodeAuthFixture(t, filepath.Join(homeDir, ".local", "share", "opencode", "auth.json"), `{"github-copilot":`)
			},
			wantErr: "parse opencode auth file",
		},
		{
			name: "custom OPENCODE_DATA_DIR",
			setup: func(t *testing.T, homeDir string) {
				t.Helper()
				customDir := filepath.Join(homeDir, "custom-opencode")
				t.Setenv("OPENCODE_DATA_DIR", customDir)
				// Ensure custom path is used instead of the default home path.
				writeOpenCodeAuthFixture(t, filepath.Join(homeDir, ".local", "share", "opencode", "auth.json"), `{"github-copilot":{"oauth_token":"home-token"}}`)
				writeOpenCodeAuthFixture(t, filepath.Join(customDir, "auth.json"), `{"github-copilot":{"oauth_token":"custom-token"}}`)
			},
			wantToken: "custom-token",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			homeDir := t.TempDir()
			t.Setenv("HOME", homeDir)
			t.Setenv("OPENCODE_DATA_DIR", "")

			tc.setup(t, homeDir)

			got, err := readOpenCodeAuth()
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("unexpected error: got %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("readOpenCodeAuth() unexpected error: %v", err)
			}
			if got != tc.wantToken {
				t.Fatalf("token: got %q, want %q", got, tc.wantToken)
			}
		})
	}
}

func TestCalculateQuotaProjection(t *testing.T) {
	tests := []struct {
		name          string
		used          int
		entitlement   int
		now           time.Time
		wantProjected int
		wantDaysLeft  int
		wantPace      string
	}{
		{
			name:          "normal mid-month",
			used:          100,
			entitlement:   400,
			now:           time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC),
			wantProjected: 207,
			wantDaysLeft:  16,
			wantPace:      "on track",
		},
		{
			name:          "day-1 edge case",
			used:          10,
			entitlement:   500,
			now:           time.Date(2026, time.March, 1, 9, 0, 0, 0, time.UTC),
			wantProjected: 310,
			wantDaysLeft:  30,
			wantPace:      "on track",
		},
		{
			name:          "zero remaining used equals entitlement",
			used:          100,
			entitlement:   100,
			now:           time.Date(2026, time.February, 14, 10, 0, 0, 0, time.UTC),
			wantProjected: 100,
			wantDaysLeft:  14,
			wantPace:      "exceeding",
		},
		{
			name:          "last-day month behavior",
			used:          75,
			entitlement:   200,
			now:           time.Date(2026, time.April, 30, 23, 59, 59, 0, time.UTC),
			wantProjected: 75,
			wantDaysLeft:  0,
			wantPace:      "on track",
		},
		{
			name:          "exceeding pace",
			used:          50,
			entitlement:   100,
			now:           time.Date(2026, time.May, 10, 8, 0, 0, 0, time.UTC),
			wantProjected: 100,
			wantDaysLeft:  21,
			wantPace:      "exceeding",
		},
		{
			name:          "entitlement zero guard",
			used:          5,
			entitlement:   0,
			now:           time.Date(2026, time.May, 10, 8, 0, 0, 0, time.UTC),
			wantProjected: 0,
			wantDaysLeft:  21,
			wantPace:      "exceeding",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			projected, daysLeft, pace := calculateQuotaProjection(tc.used, tc.entitlement, tc.now)
			if projected != tc.wantProjected {
				t.Fatalf("projected: got %d, want %d", projected, tc.wantProjected)
			}
			if daysLeft != tc.wantDaysLeft {
				t.Fatalf("daysLeft: got %d, want %d", daysLeft, tc.wantDaysLeft)
			}
			if pace != tc.wantPace {
				t.Fatalf("pace: got %q, want %q", pace, tc.wantPace)
			}
		})
	}
}

func TestCopilotTokenResponseDecode(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantToken   string
		wantExpires time.Time
	}{
		{
			name:        "decodes token and expires_at",
			payload:     `{"token":"session-token","expires_at":"2026-03-01T00:00:00Z"}`,
			wantToken:   "session-token",
			wantExpires: time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var got CopilotTokenResponse
			if err := json.Unmarshal([]byte(tc.payload), &got); err != nil {
				t.Fatalf("json.Unmarshal error: %v", err)
			}
			if got.Token != tc.wantToken {
				t.Fatalf("token: got %q, want %q", got.Token, tc.wantToken)
			}
			if !got.ExpiresAt.Equal(tc.wantExpires) {
				t.Fatalf("expires_at: got %v, want %v", got.ExpiresAt, tc.wantExpires)
			}
		})
	}
}

func TestCopilotUserResponseDecodeAndUsageGuard(t *testing.T) {
	now := time.Date(2026, time.February, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name                 string
		payload              string
		wantRemaining        int
		wantEntitlement      int
		wantResetDate        string
		wantGuardedUsed      int
		wantGuardedProjected int
		wantUsedPercent      float64
		wantProjectedPercent float64
	}{
		{
			name:                 "decodes full payload",
			payload:              `{"quota_snapshots":{"premium_interactions":{"remaining":120,"entitlement":300,"percent_remaining":40.0}},"quota_reset_date":"2026-02-28"}`,
			wantRemaining:        120,
			wantEntitlement:      300,
			wantResetDate:        "2026-02-28",
			wantGuardedUsed:      180,
			wantGuardedProjected: 300,
			wantUsedPercent:      60.0,
			wantProjectedPercent: 100.0,
		},
		{
			name:                 "missing fields with zero entitlement guard",
			payload:              `{"quota_snapshots":{"premium_interactions":{"remaining":42}}}`,
			wantRemaining:        42,
			wantEntitlement:      0,
			wantResetDate:        "",
			wantGuardedUsed:      0,
			wantGuardedProjected: 0,
			wantUsedPercent:      0.0,
			wantProjectedPercent: 0.0,
		},
		{
			name:                 "explicit zero entitlement",
			payload:              `{"quota_snapshots":{"premium_interactions":{"remaining":0,"entitlement":0}},"quota_reset_date":"2026-02-28"}`,
			wantRemaining:        0,
			wantEntitlement:      0,
			wantResetDate:        "2026-02-28",
			wantGuardedUsed:      0,
			wantGuardedProjected: 0,
			wantUsedPercent:      0.0,
			wantProjectedPercent: 0.0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var got CopilotUserResponse
			if err := json.Unmarshal([]byte(tc.payload), &got); err != nil {
				t.Fatalf("json.Unmarshal error: %v", err)
			}

			premium := got.QuotaSnapshots.PremiumInteractions
			if premium.Remaining != tc.wantRemaining {
				t.Fatalf("remaining: got %d, want %d", premium.Remaining, tc.wantRemaining)
			}
			if premium.Entitlement != tc.wantEntitlement {
				t.Fatalf("entitlement: got %d, want %d", premium.Entitlement, tc.wantEntitlement)
			}
			if got.QuotaResetDate != tc.wantResetDate {
				t.Fatalf("quota_reset_date: got %q, want %q", got.QuotaResetDate, tc.wantResetDate)
			}

			entitlement := premium.Entitlement
			if entitlement < 0 {
				entitlement = 0
			}
			used := entitlement - premium.Remaining
			if used < 0 {
				used = 0
			}
			projected, _, _ := calculateQuotaProjection(used, entitlement, now)

			usedPercent := 0.0
			projectedPercent := 0.0
			if entitlement > 0 {
				usedPercent = (float64(used) / float64(entitlement)) * 100
				projectedPercent = (float64(projected) / float64(entitlement)) * 100
			}

			if used != tc.wantGuardedUsed {
				t.Fatalf("guarded used: got %d, want %d", used, tc.wantGuardedUsed)
			}
			if projected != tc.wantGuardedProjected {
				t.Fatalf("guarded projected: got %d, want %d", projected, tc.wantGuardedProjected)
			}
			if usedPercent != tc.wantUsedPercent {
				t.Fatalf("guarded used percent: got %v, want %v", usedPercent, tc.wantUsedPercent)
			}
			if projectedPercent != tc.wantProjectedPercent {
				t.Fatalf("guarded projected percent: got %v, want %v", projectedPercent, tc.wantProjectedPercent)
			}
		})
	}
}

func TestFetchOpenCodeVersion(t *testing.T) {
	originalRunner := openCodeVersionCommandRunner
	defer func() {
		openCodeVersionCommandRunner = originalRunner
	}()

	tests := []struct {
		name     string
		output   []byte
		runErr   error
		want     string
		wantErr  string
		wantName string
		wantArgs []string
	}{
		{
			name:     "standard version output",
			output:   []byte("opencode 0.4.2\n"),
			want:     "0.4.2",
			wantName: "opencode",
			wantArgs: []string{"version"},
		},
		{
			name:     "unexpected format",
			output:   []byte("version=0.4.2 build=abc123\n"),
			want:     "0.4.2",
			wantName: "opencode",
			wantArgs: []string{"version"},
		},
		{
			name:     "command failure wrapping",
			runErr:   errors.New("boom"),
			wantErr:  "run opencode --version: boom",
			wantName: "opencode",
			wantArgs: []string{"version"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var calls [][]string
			openCodeVersionCommandRunner = func(_ context.Context, name string, args ...string) ([]byte, error) {
				if name != tc.wantName {
					t.Fatalf("command name: got %q, want %q", name, tc.wantName)
				}
				calls = append(calls, append([]string(nil), args...))
				if tc.runErr != nil {
					return nil, tc.runErr
				}
				if got, want := strings.Join(args, "\x00"), strings.Join(tc.wantArgs, "\x00"); got != want {
					t.Fatalf("command args: got %q, want %q", args, tc.wantArgs)
				}
				return append([]byte(nil), tc.output...), nil
			}

			got, err := fetchOpenCodeVersion()
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("unexpected error: got %q, want substring %q", err.Error(), tc.wantErr)
				}
				if tc.runErr != nil {
					if len(calls) != 2 {
						t.Fatalf("expected 2 calls, got %d", len(calls))
					}
					if got, want := strings.Join(calls[0], "\x00"), strings.Join([]string{"version"}, "\x00"); got != want {
						t.Fatalf("first call args: got %q, want %q", calls[0], []string{"version"})
					}
					if got, want := strings.Join(calls[1], "\x00"), strings.Join([]string{"--version"}, "\x00"); got != want {
						t.Fatalf("second call args: got %q, want %q", calls[1], []string{"--version"})
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("fetchOpenCodeVersion() unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("version: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFetchOpenCodeVersionFallback(t *testing.T) {
	originalRunner := openCodeVersionCommandRunner
	defer func() {
		openCodeVersionCommandRunner = originalRunner
	}()

	t.Run("falls back to --version when version output is not parseable", func(t *testing.T) {
		var calls [][]string
		openCodeVersionCommandRunner = func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "opencode" {
				t.Fatalf("expected command opencode, got %q", name)
			}
			calls = append(calls, append([]string(nil), args...))
			if len(calls) == 1 {
				return []byte("OpenCode CLI\n"), nil
			}
			if len(calls) == 2 {
				return []byte("v0.9.1\n"), nil
			}
			return nil, errors.New("too many calls")
		}

		got, err := fetchOpenCodeVersion()
		if err != nil {
			t.Fatalf("fetchOpenCodeVersion() unexpected error: %v", err)
		}
		if got != "0.9.1" {
			t.Fatalf("version: got %q, want %q", got, "0.9.1")
		}

		if len(calls) != 2 {
			t.Fatalf("expected 2 calls, got %d", len(calls))
		}
		if got, want := strings.Join(calls[0], "\x00"), strings.Join([]string{"version"}, "\x00"); got != want {
			t.Fatalf("first call args: got %q, want %q", calls[0], []string{"version"})
		}
		if got, want := strings.Join(calls[1], "\x00"), strings.Join([]string{"--version"}, "\x00"); got != want {
			t.Fatalf("second call args: got %q, want %q", calls[1], []string{"--version"})
		}
	})

	t.Run("falls back to --version when version command errors", func(t *testing.T) {
		var calls [][]string
		openCodeVersionCommandRunner = func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "opencode" {
				t.Fatalf("expected command opencode, got %q", name)
			}
			calls = append(calls, append([]string(nil), args...))
			if len(calls) == 1 {
				return nil, errors.New("version subcommand missing")
			}
			if len(calls) == 2 {
				return []byte("1.2.6\n"), nil
			}
			return nil, errors.New("too many calls")
		}

		got, err := fetchOpenCodeVersion()
		if err != nil {
			t.Fatalf("fetchOpenCodeVersion() unexpected error: %v", err)
		}
		if got != "1.2.6" {
			t.Fatalf("version: got %q, want %q", got, "1.2.6")
		}
		if len(calls) != 2 {
			t.Fatalf("expected 2 calls, got %d", len(calls))
		}
		if got, want := strings.Join(calls[0], "\x00"), strings.Join([]string{"version"}, "\x00"); got != want {
			t.Fatalf("first call args: got %q, want %q", calls[0], []string{"version"})
		}
		if got, want := strings.Join(calls[1], "\x00"), strings.Join([]string{"--version"}, "\x00"); got != want {
			t.Fatalf("second call args: got %q, want %q", calls[1], []string{"--version"})
		}
	})

	t.Run("returns parse error when neither command has a version", func(t *testing.T) {
		openCodeVersionCommandRunner = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("OpenCode CLI\n"), nil
		}

		_, err := fetchOpenCodeVersion()
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "unable to parse opencode version output") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestParseCcusageOutput(t *testing.T) {
	t.Run("parses pure json", func(t *testing.T) {
		output := []byte(`{"totals":{"totalCost":1.5,"totalTokens":2000},"daily":[{"date":"2026-02-19","totalCost":0.5,"totalTokens":1000}]}`)
		parsed, err := parseCcusageOutput(output)
		if err != nil {
			t.Fatalf("parseCcusageOutput() unexpected error: %v", err)
		}
		if parsed.Totals.TotalCost != 1.5 || parsed.Totals.TotalTokens != 2000 {
			t.Fatalf("unexpected totals: %+v", parsed.Totals)
		}
	})

	t.Run("parses json with npx prefix lines", func(t *testing.T) {
		output := []byte("Need to install the following packages:\n@ccusage/opencode@latest\n{\"totals\":{\"totalCost\":2.75,\"totalTokens\":3000},\"daily\":[]}\n")
		parsed, err := parseCcusageOutput(output)
		if err != nil {
			t.Fatalf("parseCcusageOutput() unexpected error: %v", err)
		}
		if parsed.Totals.TotalCost != 2.75 || parsed.Totals.TotalTokens != 3000 {
			t.Fatalf("unexpected totals: %+v", parsed.Totals)
		}
	})

	t.Run("returns error when no json present", func(t *testing.T) {
		_, err := parseCcusageOutput([]byte("@ccusage/opencode@latest\n"))
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestFetchOpenCodeDataMergesAllSources(t *testing.T) {
	originalQuota := fetchCopilotQuotaFunc
	originalCcusage := fetchOpenCodeCcusageFunc
	originalVersion := fetchOpenCodeVersionFunc
	defer func() {
		fetchCopilotQuotaFunc = originalQuota
		fetchOpenCodeCcusageFunc = originalCcusage
		fetchOpenCodeVersionFunc = originalVersion
	}()

	sessionReset := time.Date(2026, time.February, 21, 9, 0, 0, 0, time.UTC)
	weeklyReset := time.Date(2026, time.February, 28, 9, 0, 0, 0, time.UTC)

	fetchCopilotQuotaFunc = func() (DashboardData, error) {
		return DashboardData{
			ProviderID:       ProviderOpenCode,
			SessionUtil:      25.0,
			SessionResets:    sessionReset,
			HasSessionData:   true,
			WeeklyUtil:       60.0,
			WeeklyResets:     weeklyReset,
			HasWeeklyData:    true,
			QuotaUsed:        250,
			QuotaEntitlement: 1000,
			QuotaProjected:   500,
			QuotaDaysLeft:    12,
			QuotaPace:        "on track",
		}, nil
	}

	fetchOpenCodeCcusageFunc = func() (*costResult, error) {
		return &costResult{
			dailyCost:      1.25,
			dailyTokens:    1234,
			hasDailyData:   true,
			monthlyCost:    9.5,
			monthlyTokens:  5678,
			hasMonthlyData: true,
		}, nil
	}

	fetchOpenCodeVersionFunc = func() (string, error) {
		return "0.4.2", nil
	}

	got := fetchOpenCodeData()

	if got.ProviderID != ProviderOpenCode {
		t.Fatalf("provider: got %q, want %q", got.ProviderID, ProviderOpenCode)
	}
	if !got.HasSessionData || got.SessionUtil != 25.0 || !got.SessionResets.Equal(sessionReset) {
		t.Fatalf("session data mismatch: %+v", got)
	}
	if !got.HasWeeklyData || got.WeeklyUtil != 60.0 || !got.WeeklyResets.Equal(weeklyReset) {
		t.Fatalf("weekly data mismatch: %+v", got)
	}
	if got.QuotaUsed != 250 || got.QuotaEntitlement != 1000 || got.QuotaProjected != 500 || got.QuotaDaysLeft != 12 || got.QuotaPace != "on track" {
		t.Fatalf("quota data mismatch: %+v", got)
	}
	if !got.HasCostData || got.DailyCost != 1.25 || got.DailyTokens != 1234 {
		t.Fatalf("daily cost data mismatch: %+v", got)
	}
	if !got.HasMonthlyData || got.MonthlyCost != 9.5 || got.MonthlyTokens != 5678 {
		t.Fatalf("monthly data mismatch: %+v", got)
	}
	if !got.HasVersionData || got.ProviderVersion != "0.4.2" {
		t.Fatalf("version data mismatch: %+v", got)
	}
	if got.LastUpdated.IsZero() {
		t.Fatal("expected LastUpdated to be set")
	}
	if len(got.Errors) != 0 {
		t.Fatalf("expected no errors, got %v", got.Errors)
	}
}

func writeOpenCodeAuthFixture(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
