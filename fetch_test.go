package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
