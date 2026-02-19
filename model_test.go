package main

import (
	"errors"
	"strings"
	"testing"
	"time"

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

func TestManualRefreshQueuesWhenFetchIsInFlight(t *testing.T) {
	originalFetch := fetchAllDataForProviderFunc
	defer func() {
		fetchAllDataForProviderFunc = originalFetch
	}()

	fetchCalls := 0
	fetchAllDataForProviderFunc = func(provider ProviderID) DashboardData {
		fetchCalls++
		return DashboardData{
			ProviderID:  provider,
			LastUpdated: time.Date(2026, time.February, 19, 12, 0, 0, 0, time.UTC),
		}
	}

	m := model{
		state:            StateDashboard,
		selectedProvider: ProviderCodex,
		fetching:         true,
		data:             DashboardData{ProviderID: ProviderCodex},
	}

	nextModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd != nil {
		t.Fatal("expected no new fetch command while fetch is in flight")
	}

	afterKey := nextModel.(model)
	if !afterKey.queuedRefresh {
		t.Fatal("expected refresh to be queued")
	}

	fetched := dataFetchedMsg{
		provider: ProviderCodex,
		data: DashboardData{
			ProviderID:  ProviderCodex,
			LastUpdated: time.Date(2026, time.February, 19, 12, 1, 0, 0, time.UTC),
		},
	}
	nextModel, cmd = afterKey.Update(fetched)
	if cmd == nil {
		t.Fatal("expected queued refresh to start after in-flight fetch completes")
	}

	afterFetch := nextModel.(model)
	if !afterFetch.fetching {
		t.Fatal("expected fetching=true while queued refresh is dispatched")
	}
	if afterFetch.queuedRefresh {
		t.Fatal("queued refresh should be consumed")
	}

	msg := cmd()
	if _, ok := msg.(dataFetchedMsg); !ok {
		t.Fatalf("expected dataFetchedMsg, got %T", msg)
	}
	if fetchCalls != 1 {
		t.Fatalf("expected one queued fetch call, got %d", fetchCalls)
	}
}

func TestTickDoesNotOverlapWhenFetching(t *testing.T) {
	m := model{
		state:            StateDashboard,
		selectedProvider: ProviderClaude,
		fetching:         true,
		secondsToRefresh: 0,
	}

	nextModel, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("expected tick command")
	}

	after := nextModel.(model)
	if after.secondsToRefresh != 0 {
		t.Fatalf("secondsToRefresh changed while fetching: got %d want %d", after.secondsToRefresh, 0)
	}
	if !after.fetching {
		t.Fatal("fetching should remain true")
	}
}

func TestWarmUpErrorPersistsAfterSuccessfulRefresh(t *testing.T) {
	m := model{
		state:            StateDashboard,
		selectedProvider: ProviderClaude,
		data:             DashboardData{ProviderID: ProviderClaude},
		warmingUp:        true,
	}

	nextModel, cmd := m.Update(warmupFinishedMsg{provider: ProviderClaude, err: errors.New("warm failed")})
	if cmd == nil {
		t.Fatal("expected refresh command after warm-up completion")
	}
	afterWarmup := nextModel.(model)
	if len(afterWarmup.data.Errors) == 0 {
		t.Fatal("expected warm-up error to be present")
	}

	fetched := dataFetchedMsg{
		provider: ProviderClaude,
		data: DashboardData{
			ProviderID:  ProviderClaude,
			LastUpdated: time.Date(2026, time.February, 19, 13, 0, 0, 0, time.UTC),
		},
	}
	nextModel, _ = afterWarmup.Update(fetched)
	afterFetch := nextModel.(model)

	if len(afterFetch.data.Errors) == 0 {
		t.Fatal("expected warm-up error to persist after refresh merge")
	}
	if got := afterFetch.data.Errors[len(afterFetch.data.Errors)-1]; !strings.Contains(got, "warm-up: warm failed") {
		t.Fatalf("unexpected merged error tail: %q", got)
	}
}
