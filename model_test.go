package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRenderDashboardFooterContainsWarmUpWithoutCountdown(t *testing.T) {
	m := model{
		state:            StateDashboard,
		selectedProvider: ProviderCodex,
		width:            120,
		height:           40,
		data: DashboardData{
			ProviderID: ProviderCodex,
		},
	}

	view := m.renderDashboardView()
	if !strings.Contains(view, "w: warm up") {
		t.Fatalf("expected footer to contain warm-up keybind, got:\n%s", view)
	}
	if strings.Contains(view, "Next:") {
		t.Fatalf("footer should not contain countdown, got:\n%s", view)
	}
}

func TestWarmUpKeyTriggersWarmUpThenRefresh(t *testing.T) {
	originalWarmUp := warmUpProviderFunc
	originalFetch := fetchAllDataForProviderFunc
	defer func() {
		warmUpProviderFunc = originalWarmUp
		fetchAllDataForProviderFunc = originalFetch
	}()

	var warmUpCalls int
	warmUpProviderFunc = func(provider ProviderID) error {
		warmUpCalls++
		if provider != ProviderCodex {
			t.Fatalf("expected codex warm-up, got %q", provider)
		}
		return nil
	}
	fetchAllDataForProviderFunc = func(provider ProviderID) DashboardData {
		return DashboardData{ProviderID: provider}
	}

	m := model{
		state:            StateDashboard,
		selectedProvider: ProviderCodex,
		data:             DashboardData{ProviderID: ProviderCodex},
	}

	nextModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if cmd == nil {
		t.Fatal("expected warm-up command")
	}

	afterKey := nextModel.(model)
	if !afterKey.warmingUp {
		t.Fatal("expected warmingUp=true after pressing w")
	}

	msg := cmd()
	warmupMsg, ok := msg.(warmupFinishedMsg)
	if !ok {
		t.Fatalf("expected warmupFinishedMsg, got %T", msg)
	}
	if warmupMsg.provider != ProviderCodex {
		t.Fatalf("expected codex warm-up message, got %q", warmupMsg.provider)
	}
	if warmupMsg.err != nil {
		t.Fatalf("expected no warm-up error, got %v", warmupMsg.err)
	}
	if warmUpCalls != 1 {
		t.Fatalf("expected 1 warm-up call, got %d", warmUpCalls)
	}

	nextModel, refreshCmd := afterKey.Update(warmupMsg)
	if refreshCmd == nil {
		t.Fatal("expected refresh command after warm-up")
	}

	afterWarmup := nextModel.(model)
	if afterWarmup.warmingUp {
		t.Fatal("expected warmingUp=false after warm-up completion")
	}
	if !afterWarmup.fetching {
		t.Fatal("expected fetching=true after warm-up completion")
	}

	refreshMsg := refreshCmd()
	dataMsg, ok := refreshMsg.(dataFetchedMsg)
	if !ok {
		t.Fatalf("expected dataFetchedMsg, got %T", refreshMsg)
	}
	if dataMsg.provider != ProviderCodex {
		t.Fatalf("expected codex refresh, got %q", dataMsg.provider)
	}
}

func TestWarmUpFinishedErrorAppendsErrorAndRefreshes(t *testing.T) {
	originalFetch := fetchAllDataForProviderFunc
	defer func() {
		fetchAllDataForProviderFunc = originalFetch
	}()

	fetchAllDataForProviderFunc = func(provider ProviderID) DashboardData {
		return DashboardData{ProviderID: provider}
	}

	m := model{
		state:            StateDashboard,
		selectedProvider: ProviderClaude,
		data:             DashboardData{ProviderID: ProviderClaude},
		warmingUp:        true,
	}

	nextModel, cmd := m.Update(warmupFinishedMsg{provider: ProviderClaude, err: errors.New("failed")})
	if cmd == nil {
		t.Fatal("expected refresh command even when warm-up fails")
	}

	after := nextModel.(model)
	if after.warmingUp {
		t.Fatal("expected warmingUp=false")
	}
	if len(after.data.Errors) == 0 {
		t.Fatal("expected warm-up error to be appended")
	}
	if got := after.data.Errors[len(after.data.Errors)-1]; !strings.Contains(got, "warm-up: ") {
		t.Fatalf("unexpected error entry: %q", got)
	}

	refreshMsg := cmd()
	if _, ok := refreshMsg.(dataFetchedMsg); !ok {
		t.Fatalf("expected dataFetchedMsg, got %T", refreshMsg)
	}
}

func TestWarmUpKeyIgnoredWhileWarmUpRunning(t *testing.T) {
	m := model{
		state:            StateDashboard,
		selectedProvider: ProviderCodex,
		warmingUp:        true,
	}

	nextModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	if cmd != nil {
		t.Fatal("expected nil command while warm-up is already running")
	}
	if !nextModel.(model).warmingUp {
		t.Fatal("warmingUp should remain true")
	}
}
