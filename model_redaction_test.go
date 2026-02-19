package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWarmUpFinishedErrorIsSanitizedForUI(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", homeDir)

	secretPath := filepath.Join(homeDir, ".codex", "sessions", "session.jsonl")
	m := model{
		state:            StateDashboard,
		selectedProvider: ProviderClaude,
		data:             DashboardData{ProviderID: ProviderClaude},
		warmingUp:        true,
	}

	next, _ := m.Update(warmupFinishedMsg{
		provider: ProviderClaude,
		err:      fmt.Errorf("Bearer warmup-secret-123 token=warmup-query-123 path=%s", secretPath),
	})

	after := next.(model)
	if len(after.data.Errors) == 0 {
		t.Fatal("expected warm-up error entry")
	}
	last := after.data.Errors[len(after.data.Errors)-1]
	if !strings.Contains(last, "warm-up:") {
		t.Fatalf("expected warm-up prefix, got %q", last)
	}
	for _, leaked := range []string{"warmup-secret-123", "warmup-query-123", homeDir} {
		if strings.Contains(last, leaked) {
			t.Fatalf("warm-up error leaked %q: %q", leaked, last)
		}
	}
}

func TestProviderSelectionConfigErrorIsSanitizedForUI(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", homeDir)

	blockingConfigDir := filepath.Join(homeDir, ".llm-status")
	if err := os.MkdirAll(filepath.Dir(blockingConfigDir), 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if err := os.WriteFile(blockingConfigDir, []byte("not-a-dir"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	m := model{
		state:            StateChooseProvider,
		providerMenuIdx:  providerIndex(ProviderClaude),
		secondsToRefresh: refreshInterval,
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	after := next.(model)
	if len(after.data.Errors) == 0 {
		t.Fatal("expected config error entry")
	}
	last := after.data.Errors[len(after.data.Errors)-1]
	if !strings.Contains(last, "config:") {
		t.Fatalf("expected config prefix, got %q", last)
	}
	if strings.Contains(last, homeDir) {
		t.Fatalf("config error leaked absolute home path: %q", last)
	}
}
