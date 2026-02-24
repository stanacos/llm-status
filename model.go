package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	refreshInterval     = 60
	fastRefreshInterval = 15
	maxFetchDuration    = 2 * time.Minute
	staleResetThreshold = 5 * time.Minute
	nearResetThreshold  = 2 * time.Minute
	minWidth            = 56
	minHeight           = 20
)

var (
	fetchAllDataForProviderFunc = fetchAllDataForProvider
	warmUpProviderFunc          = warmUpProvider
)

type model struct {
	state            AppState
	selectedProvider ProviderID
	providerMenuIdx  int

	data             DashboardData
	spinner          spinner.Model
	fetching         bool
	fetchStartedAt   time.Time
	warmingUp        bool
	queuedRefresh    bool
	width            int
	height           int
	secondsToRefresh int
}

func newModel() model {
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    80 * time.Millisecond,
	}
	s.Style = lipgloss.NewStyle().Foreground(colorCyan)

	m := model{
		state:            StateChooseProvider,
		providerMenuIdx:  0,
		spinner:          s,
		secondsToRefresh: refreshInterval,
	}

	provider, err := loadLastProvider()
	if err != nil {
		appendSanitizedErrorFromErr(&m.data.Errors, "config: ", err)
	}
	if err := checkNpxAvailable(); err != nil {
		appendSanitizedErrorFromErr(&m.data.Errors, "", err)
	}
	if isValidProvider(provider) {
		m.state = StateDashboard
		m.selectedProvider = provider
		m.providerMenuIdx = providerIndex(provider)
		m.fetching = true
		m.data.ProviderID = provider
	}

	return m
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick, doTick()}
	if m.state == StateDashboard {
		cmds = append(cmds, fetchAllDataCmd(m.selectedProvider))
	}
	return tea.Batch(cmds...)
}

func doTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func fetchAllDataCmd(provider ProviderID) tea.Cmd {
	return func() tea.Msg {
		data := fetchAllDataForProviderFunc(provider)
		return dataFetchedMsg{provider: provider, data: data}
	}
}

func warmupProviderCmd(provider ProviderID) tea.Cmd {
	return func() tea.Msg {
		err := warmUpProviderFunc(provider)
		return warmupFinishedMsg{provider: provider, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

		if m.state == StateChooseProvider {
			switch msg.String() {
			case "up", "k":
				providers := allProviders()
				if len(providers) == 0 {
					return m, nil
				}
				m.providerMenuIdx--
				if m.providerMenuIdx < 0 {
					m.providerMenuIdx = len(providers) - 1
				}
				return m, nil
			case "down", "j":
				providers := allProviders()
				if len(providers) == 0 {
					return m, nil
				}
				m.providerMenuIdx++
				if m.providerMenuIdx >= len(providers) {
					m.providerMenuIdx = 0
				}
				return m, nil
			case "enter":
				providers := allProviders()
				if len(providers) == 0 {
					return m, nil
				}
				chosen := providers[m.providerMenuIdx].ID
				m.state = StateDashboard
				m.selectedProvider = chosen
				m.fetching = true
				m.fetchStartedAt = time.Now()
				m.queuedRefresh = false
				m.secondsToRefresh = refreshInterval
				m.data = DashboardData{ProviderID: chosen}
				if err := saveLastProvider(chosen); err != nil {
					appendSanitizedErrorFromErr(&m.data.Errors, "config: ", err)
				}
				return m, fetchAllDataCmd(chosen)
			}
			return m, nil
		}

		switch msg.String() {
		case "r":
			if m.fetching {
				m.queuedRefresh = true
				m.secondsToRefresh = refreshInterval
				return m, nil
			}
			m.fetching = true
			m.fetchStartedAt = time.Now()
			m.secondsToRefresh = refreshInterval
			return m, fetchAllDataCmd(m.selectedProvider)
		case "w":
			if m.warmingUp {
				return m, nil
			}
			m.warmingUp = true
			return m, warmupProviderCmd(m.selectedProvider)
		case "p":
			m.state = StateChooseProvider
			m.fetching = false
			m.fetchStartedAt = time.Time{}
			m.queuedRefresh = false
			m.providerMenuIdx = providerIndex(m.selectedProvider)
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		if m.state != StateDashboard {
			return m, doTick()
		}
		if m.fetching {
			if !m.fetchStartedAt.IsZero() && time.Since(m.fetchStartedAt) > maxFetchDuration {
				m.fetching = false
				m.fetchStartedAt = time.Time{}
				appendSanitizedError(&m.data.Errors, "Fetch timed out — press r to retry")
			}
			return m, doTick()
		}

		m.secondsToRefresh--

		// Accelerate refresh when near/past a reset window.
		if m.data.HasSessionData && !m.data.SessionResets.IsZero() &&
			time.Until(m.data.SessionResets) < nearResetThreshold &&
			m.secondsToRefresh > fastRefreshInterval {
			m.secondsToRefresh = fastRefreshInterval
		}

		if m.secondsToRefresh <= 0 {
			m.fetching = true
			m.fetchStartedAt = time.Now()
			m.secondsToRefresh = refreshInterval
			return m, tea.Batch(doTick(), fetchAllDataCmd(m.selectedProvider))
		}
		return m, doTick()

	case dataFetchedMsg:
		if m.state != StateDashboard || msg.provider != m.selectedProvider {
			return m, nil
		}
		if m.data.ProviderID != msg.provider {
			m.data = DashboardData{ProviderID: msg.provider}
		}
		m.data = mergeData(m.data, msg.data)
		if m.queuedRefresh {
			m.queuedRefresh = false
			m.fetching = true
			m.fetchStartedAt = time.Now()
			m.secondsToRefresh = refreshInterval
			return m, fetchAllDataCmd(m.selectedProvider)
		}
		m.fetching = false
		m.fetchStartedAt = time.Time{}
		return m, nil

	case warmupFinishedMsg:
		m.warmingUp = false
		if msg.provider != m.selectedProvider {
			return m, nil
		}
		if msg.err != nil {
			appendSanitizedErrorFromErr(&m.data.Errors, "warm-up: ", msg.err)
		}
		if m.state != StateDashboard {
			return m, nil
		}
		if m.fetching {
			m.queuedRefresh = true
			m.secondsToRefresh = refreshInterval
			return m, nil
		}
		m.fetching = true
		m.secondsToRefresh = refreshInterval
		return m, fetchAllDataCmd(m.selectedProvider)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// contentWidth returns the usable width inside the outer border (border + padding).
func (m model) contentWidth() int {
	// 2 border chars + 4 horizontal padding (2 each side)
	cw := m.width - 6
	if cw < 34 {
		cw = 34
	}
	return cw
}

// barWidth returns the progress bar width, proportional to content width.
func (m model) barWidth() int {
	bw := m.contentWidth() * 40 / 100
	if bw < 10 {
		bw = 10
	}
	return bw
}

func (m model) View() string {
	if m.width < minWidth || m.height < minHeight {
		if m.width == 0 && m.height == 0 {
			return "" // Initial render before WindowSizeMsg
		}
		msg := lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true).
			Render("Terminal too small (need 56x20)")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg,
			lipgloss.WithWhitespaceBackground(colorBackground))
	}

	if m.state == StateChooseProvider {
		return m.renderProviderSelectionView()
	}
	return m.renderDashboardView()
}

func (m model) renderProviderSelectionView() string {
	cw := m.contentWidth()

	title := lipgloss.NewStyle().
		Foreground(colorPurple).
		Bold(true).
		Width(cw).
		Align(lipgloss.Center).
		Render("SELECT PROVIDER")

	description := lipgloss.NewStyle().
		Foreground(colorComment).
		Width(cw).
		Align(lipgloss.Center).
		Render("Use ↑/↓ (or j/k) and Enter")

	providers := allProviders()
	lines := make([]string, 0, len(providers))
	for i, provider := range providers {
		lineStyle := lipgloss.NewStyle().
			Foreground(colorForeground).
			Width(cw).
			Align(lipgloss.Center)

		prefix := "  "
		if i == m.providerMenuIdx {
			prefix = "› "
			lineStyle = lineStyle.Foreground(colorCyan).Bold(true)
		}

		lines = append(lines, lineStyle.Render(prefix+provider.DisplayName))
	}

	hints := lipgloss.NewStyle().
		Foreground(colorComment).
		Width(cw).
		Align(lipgloss.Center).
		Render("q: quit")

	parts := []string{title, "", description, ""}
	parts = append(parts, lines...)
	parts = append(parts, "", hints)

	inner := lipgloss.JoinVertical(lipgloss.Center, parts...)

	frameWidth := m.width - 2
	frameHeight := m.height - 2
	outerFrame := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPurple).
		Padding(0, 2).
		Width(frameWidth).
		Height(frameHeight).
		Render(lipgloss.PlaceVertical(frameHeight, lipgloss.Center, inner))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, outerFrame,
		lipgloss.WithWhitespaceBackground(colorBackground))
}

func (m model) renderDashboardView() string {
	cw := m.contentWidth()
	meta := providerMeta(m.selectedProvider)

	// Header
	header := lipgloss.NewStyle().
		Foreground(colorPurple).
		Bold(true).
		Width(cw).
		Align(lipgloss.Center).
		Render(meta.HeaderTitle)

	// Version line
	var versionLine string
	if m.data.HasVersionData {
		versionLine = lipgloss.NewStyle().
			Foreground(colorComment).
			Width(cw).
			Align(lipgloss.Center).
			Render(fmt.Sprintf("%s v%s", meta.VersionPrefix, m.data.ProviderVersion))
	}

	// Status line
	var statusLine string
	if m.fetching {
		statusLine = lipgloss.NewStyle().
			Foreground(colorCyan).
			Width(cw).
			Align(lipgloss.Center).
			Render(m.spinner.View() + " Refreshing...")
	} else if !m.data.LastUpdated.IsZero() {
		statusLine = lipgloss.NewStyle().
			Foreground(colorComment).
			Width(cw).
			Align(lipgloss.Center).
			Render("Last updated: " + m.data.LastUpdated.Format("15:04:05"))
	}

	// Error line
	var errorLine string
	if len(m.data.Errors) > 0 {
		errorLine = lipgloss.NewStyle().
			Foreground(colorComment).
			Italic(true).
			Width(cw).
			Align(lipgloss.Center).
			Render(m.data.Errors[len(m.data.Errors)-1])
	}

	// Panels
	sessionPanel := m.renderSessionPanel()
	weeklyPanel := m.renderWeeklyPanel()
	todayPanel := m.renderTodayPanel()
	monthlyPanel := m.renderMonthlyPanel()

	// Footer
	footer := lipgloss.NewStyle().
		Foreground(colorComment).
		Width(cw).
		Align(lipgloss.Center).
		Render("q: quit • p: providers • r: refresh • w: warm up")

	parts := []string{header}
	if versionLine != "" {
		parts = append(parts, versionLine)
	}
	parts = append(parts, statusLine)
	if errorLine != "" {
		parts = append(parts, errorLine)
	}
	parts = append(parts, "", sessionPanel, "", weeklyPanel, "", todayPanel, "", monthlyPanel, "", footer)

	inner := lipgloss.JoinVertical(lipgloss.Center, parts...)

	frameWidth := m.width - 2
	frameHeight := m.height - 2
	outerFrame := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPurple).
		Padding(0, 2).
		Width(frameWidth).
		Height(frameHeight).
		Render(lipgloss.PlaceVertical(frameHeight, lipgloss.Center, inner))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, outerFrame,
		lipgloss.WithWhitespaceBackground(colorBackground))
}

func (m model) renderSessionPanel() string {
	cw := m.contentWidth()
	title := providerMeta(m.selectedProvider).SessionLabel
	if !m.data.HasSessionData {
		content := lipgloss.NewStyle().Foreground(colorComment).Render("N/A")
		return renderPanel(title, content, cw)
	}

	bar := renderBrailleBar(m.data.SessionUtil, m.barWidth())
	pct := formatPercent(m.data.SessionUtil)

	if m.selectedProvider == ProviderOpenCode {
		used := fmt.Sprintf("%d / %d", m.data.QuotaUsed, m.data.QuotaEntitlement)
		resetDate := m.data.SessionResets.Local().Format("Mon 02/01")

		content := lipgloss.JoinVertical(lipgloss.Left,
			bar+" "+pct,
			renderMetricRow("Used", used, cw-4),
			renderMetricRow("Resets on", resetDate, cw-4),
			renderMetricRow("Resets at", "00:00", cw-4),
		)
		return renderPanel(title, content, cw)
	}

	resetDur := time.Until(m.data.SessionResets)
	resetStr := formatDuration(resetDur)
	if resetDur <= 0 {
		resetStr = "Resetting…"
	}
	resetAt := m.data.SessionResets.Local().Format("15:04")

	content := lipgloss.JoinVertical(lipgloss.Left,
		bar+" "+pct,
		renderMetricRow("Resets in", resetStr, cw-4),
		renderMetricRow("Resets at", resetAt, cw-4),
	)
	return renderPanel(title, content, cw)
}

func (m model) renderWeeklyPanel() string {
	cw := m.contentWidth()
	title := providerMeta(m.selectedProvider).WeeklyLabel
	if !m.data.HasWeeklyData {
		content := lipgloss.NewStyle().Foreground(colorComment).Render("N/A")
		return renderPanel(title, content, cw)
	}

	bar := renderBrailleBar(m.data.WeeklyUtil, m.barWidth())
	pct := formatPercent(m.data.WeeklyUtil)

	if m.selectedProvider == ProviderOpenCode {
		projected := fmt.Sprintf("%d / %d", m.data.QuotaProjected, m.data.QuotaEntitlement)
		daysLeft := fmt.Sprintf("%d", m.data.QuotaDaysLeft)

		content := lipgloss.JoinVertical(lipgloss.Left,
			bar+" "+pct,
			renderMetricRow("Projected", projected, cw-4),
			renderMetricRow("Days left", daysLeft, cw-4),
			renderMetricRow("Pace", m.data.QuotaPace, cw-4),
		)
		return renderPanel(title, content, cw)
	}

	resetDate := m.data.WeeklyResets.Local().Format("Mon 02/01")
	resetAt := m.data.WeeklyResets.Local().Format("15:04")

	content := lipgloss.JoinVertical(lipgloss.Left,
		bar+" "+pct,
		renderMetricRow("Resets on", resetDate, cw-4),
		renderMetricRow("Resets at", resetAt, cw-4),
	)
	return renderPanel(title, content, cw)
}

func (m model) renderTodayPanel() string {
	cw := m.contentWidth()
	costStr := "N/A"
	tokenStr := "N/A"
	if m.data.HasCostData {
		costStr = formatCost(m.data.DailyCost)
		tokenStr = formatTokens(m.data.DailyTokens)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		renderMetricRow("Cost", costStr, cw-4),
		renderMetricRow("Tokens", tokenStr, cw-4),
	)
	return renderPanel("Today", content, cw)
}

func (m model) renderMonthlyPanel() string {
	cw := m.contentWidth()
	costStr := "N/A"
	tokenStr := "N/A"
	if m.data.HasMonthlyData {
		costStr = formatCost(m.data.MonthlyCost)
		tokenStr = formatTokens(m.data.MonthlyTokens)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		renderMetricRow("Cost", costStr, cw-4),
		renderMetricRow("Tokens", tokenStr, cw-4),
	)
	return renderPanel("Last 30 Days", content, cw)
}

// mergeData merges new data into old, retaining previous values for failed sources.
func mergeData(old, new DashboardData) DashboardData {
	const maxErrorEntries = 12

	result := old
	result.ProviderID = new.ProviderID

	// Expire stale session data when reset time is well past and new fetch has no update.
	if !new.HasSessionData && old.HasSessionData && !old.SessionResets.IsZero() &&
		time.Since(old.SessionResets) > staleResetThreshold {
		result.HasSessionData = false
		result.SessionResets = time.Time{}
		result.SessionUtil = 0
	}

	// Expire stale weekly data when reset time is well past and new fetch has no update.
	if !new.HasWeeklyData && old.HasWeeklyData && !old.WeeklyResets.IsZero() &&
		time.Since(old.WeeklyResets) > staleResetThreshold {
		result.HasWeeklyData = false
		result.WeeklyResets = time.Time{}
		result.WeeklyUtil = 0
	}

	if new.HasSessionData {
		result.SessionUtil = new.SessionUtil
		result.SessionResets = new.SessionResets
		result.HasSessionData = true
	}

	if new.HasWeeklyData {
		result.WeeklyUtil = new.WeeklyUtil
		result.WeeklyResets = new.WeeklyResets
		result.HasWeeklyData = true
	}

	if new.HasSessionData || new.HasWeeklyData {
		result.QuotaUsed = new.QuotaUsed
		result.QuotaEntitlement = new.QuotaEntitlement
		result.QuotaProjected = new.QuotaProjected
		result.QuotaDaysLeft = new.QuotaDaysLeft
		result.QuotaPace = new.QuotaPace
	}

	if new.HasCostData {
		result.DailyCost = new.DailyCost
		result.DailyTokens = new.DailyTokens
		result.HasCostData = true
	}

	if new.HasMonthlyData {
		result.MonthlyCost = new.MonthlyCost
		result.MonthlyTokens = new.MonthlyTokens
		result.HasMonthlyData = true
	}

	if new.HasVersionData {
		result.ProviderVersion = new.ProviderVersion
		result.HasVersionData = true
	}

	result.LastUpdated = new.LastUpdated
	if len(old.Errors) > 0 || len(new.Errors) > 0 {
		mergedErrors := make([]string, 0, len(old.Errors)+len(new.Errors))
		mergedErrors = append(mergedErrors, old.Errors...)
		mergedErrors = append(mergedErrors, new.Errors...)
		if len(mergedErrors) > maxErrorEntries {
			mergedErrors = mergedErrors[len(mergedErrors)-maxErrorEntries:]
		}
		result.Errors = mergedErrors
	}
	return result
}
