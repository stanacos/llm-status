package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	refreshInterval = 60
	minWidth        = 56
	minHeight       = 20
)

type model struct {
	state            AppState
	selectedProvider ProviderID
	providerMenuIdx  int

	data             DashboardData
	spinner          spinner.Model
	fetching         bool
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
		m.data.Errors = append(m.data.Errors, "config: "+err.Error())
	}
	if err := checkNpxAvailable(); err != nil {
		m.data.Errors = append(m.data.Errors, err.Error())
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
		data := fetchAllDataForProvider(provider)
		return dataFetchedMsg{provider: provider, data: data}
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
				m.secondsToRefresh = refreshInterval
				m.data = DashboardData{ProviderID: chosen}
				if err := saveLastProvider(chosen); err != nil {
					m.data.Errors = append(m.data.Errors, "config: "+err.Error())
				}
				return m, fetchAllDataCmd(chosen)
			}
			return m, nil
		}

		switch msg.String() {
		case "r":
			m.fetching = true
			m.secondsToRefresh = refreshInterval
			return m, fetchAllDataCmd(m.selectedProvider)
		case "p":
			m.state = StateChooseProvider
			m.fetching = false
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

		m.secondsToRefresh--
		if m.secondsToRefresh <= 0 {
			m.fetching = true
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
		m.fetching = false
		return m, nil

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
		Render("q: quit • p: providers • r: refresh • Next: " + formatCountdown(m.secondsToRefresh))

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
	if !m.data.HasSessionData {
		content := lipgloss.NewStyle().Foreground(colorComment).Render("N/A")
		return renderPanel("Session (5h)", content, cw)
	}

	bar := renderBrailleBar(m.data.SessionUtil, m.barWidth())
	pct := formatPercent(m.data.SessionUtil)
	resetStr := formatDuration(time.Until(m.data.SessionResets))
	resetAt := m.data.SessionResets.Local().Format("15:04")

	content := lipgloss.JoinVertical(lipgloss.Left,
		bar+" "+pct,
		renderMetricRow("Resets in", resetStr, cw-4),
		renderMetricRow("Resets at", resetAt, cw-4),
	)
	return renderPanel("Session (5h)", content, cw)
}

func (m model) renderWeeklyPanel() string {
	cw := m.contentWidth()
	if !m.data.HasWeeklyData {
		content := lipgloss.NewStyle().Foreground(colorComment).Render("N/A")
		return renderPanel("Weekly (7d)", content, cw)
	}

	bar := renderBrailleBar(m.data.WeeklyUtil, m.barWidth())
	pct := formatPercent(m.data.WeeklyUtil)
	resetDate := m.data.WeeklyResets.Local().Format("Mon 02/01")
	resetAt := m.data.WeeklyResets.Local().Format("15:04")

	content := lipgloss.JoinVertical(lipgloss.Left,
		bar+" "+pct,
		renderMetricRow("Resets on", resetDate, cw-4),
		renderMetricRow("Resets at", resetAt, cw-4),
	)
	return renderPanel("Weekly (7d)", content, cw)
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

func formatCountdown(seconds int) string {
	if seconds <= 0 {
		return "now"
	}
	return formatInt(seconds) + "s"
}

// mergeData merges new data into old, retaining previous values for failed sources.
func mergeData(old, new DashboardData) DashboardData {
	result := old
	result.ProviderID = new.ProviderID

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
	result.Errors = new.Errors
	return result
}
