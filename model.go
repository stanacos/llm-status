package main

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	refreshInterval = 60
	dashboardWidth  = 54
	minWidth        = 56
	minHeight       = 20
)

type model struct {
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

	return model{
		spinner:          s,
		fetching:         true,
		secondsToRefresh: refreshInterval,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		doTick(),
		fetchAllDataCmd(),
	)
}

func doTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func fetchAllDataCmd() tea.Cmd {
	return func() tea.Msg {
		data := fetchAllData()
		return dataFetchedMsg{data: data}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.fetching = true
			m.secondsToRefresh = refreshInterval
			return m, fetchAllDataCmd()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.secondsToRefresh--
		if m.secondsToRefresh <= 0 {
			m.fetching = true
			m.secondsToRefresh = refreshInterval
			return m, tea.Batch(doTick(), fetchAllDataCmd())
		}
		return m, doTick()

	case dataFetchedMsg:
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

func (m model) View() string {
	if m.width < minWidth || m.height < minHeight {
		if m.width == 0 && m.height == 0 {
			return "" // Initial render before WindowSizeMsg
		}
		msg := lipgloss.NewStyle().
			Foreground(colorOrange).
			Bold(true).
			Render("Terminal too small (need 56×20)")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg,
			lipgloss.WithWhitespaceBackground(colorBackground))
	}

	// Header
	header := lipgloss.NewStyle().
		Foreground(colorPurple).
		Bold(true).
		Width(dashboardWidth).
		Align(lipgloss.Center).
		Render("CLAUDE CODE STATUS")

	// Status line
	var statusLine string
	if m.fetching {
		statusLine = lipgloss.NewStyle().
			Foreground(colorCyan).
			Width(dashboardWidth).
			Align(lipgloss.Center).
			Render(m.spinner.View() + " Refreshing...")
	} else if !m.data.LastUpdated.IsZero() {
		statusLine = lipgloss.NewStyle().
			Foreground(colorComment).
			Width(dashboardWidth).
			Align(lipgloss.Center).
			Render("Last updated: " + m.data.LastUpdated.Format("15:04:05"))
	}

	// Error line
	var errorLine string
	if len(m.data.Errors) > 0 {
		errorLine = lipgloss.NewStyle().
			Foreground(colorComment).
			Italic(true).
			Width(dashboardWidth).
			Align(lipgloss.Center).
			Render(m.data.Errors[len(m.data.Errors)-1])
	}

	// Panels
	sessionPanel := m.renderSessionPanel()
	weeklyPanel := m.renderWeeklyPanel()
	todayPanel := m.renderTodayPanel()

	// Footer
	footer := lipgloss.NewStyle().
		Foreground(colorComment).
		Width(dashboardWidth).
		Align(lipgloss.Center).
		Render("q: quit • r: refresh • Next: " + formatCountdown(m.secondsToRefresh))

	// Compose inner content
	parts := []string{header, "", statusLine}
	if errorLine != "" {
		parts = append(parts, errorLine)
	}
	parts = append(parts, "", sessionPanel, "", weeklyPanel, "", todayPanel, "", footer)

	inner := lipgloss.JoinVertical(lipgloss.Center, parts...)

	// Outer frame
	outerFrame := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPurple).
		Padding(1, 2).
		Render(inner)

	// Center in terminal
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, outerFrame,
		lipgloss.WithWhitespaceBackground(colorBackground))
}

func (m model) renderSessionPanel() string {
	if !m.data.HasOAuthData {
		content := lipgloss.NewStyle().Foreground(colorComment).Render("N/A")
		return renderPanel("Session (5h)", content, dashboardWidth)
	}

	bar := renderBrailleBar(m.data.SessionUtil, 20)
	pct := formatPercent(m.data.SessionUtil)
	resetStr := formatDuration(time.Until(m.data.SessionResets))

	content := lipgloss.JoinVertical(lipgloss.Left,
		bar+" "+pct,
		renderMetricRow("Resets in", resetStr, dashboardWidth-4),
	)
	return renderPanel("Session (5h)", content, dashboardWidth)
}

func (m model) renderWeeklyPanel() string {
	if !m.data.HasOAuthData {
		content := lipgloss.NewStyle().Foreground(colorComment).Render("N/A")
		return renderPanel("Weekly (7d)", content, dashboardWidth)
	}

	bar := renderBrailleBar(m.data.WeeklyUtil, 20)
	pct := formatPercent(m.data.WeeklyUtil)
	resetStr := formatDuration(time.Until(m.data.WeeklyResets))

	content := lipgloss.JoinVertical(lipgloss.Left,
		bar+" "+pct,
		renderMetricRow("Resets in", resetStr, dashboardWidth-4),
	)
	return renderPanel("Weekly (7d)", content, dashboardWidth)
}

func (m model) renderTodayPanel() string {
	costStr := "N/A"
	if m.data.HasCostData {
		costStr = formatCost(m.data.DailyCost)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		renderMetricRow("Cost", costStr, dashboardWidth-4),
		renderMetricRow("Messages", formatInt(m.data.MessageCount), dashboardWidth-4),
		renderMetricRow("Sessions", formatInt(m.data.SessionCount), dashboardWidth-4),
	)
	return renderPanel("Today", content, dashboardWidth)
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

	if new.HasOAuthData {
		result.SessionUtil = new.SessionUtil
		result.SessionResets = new.SessionResets
		result.WeeklyUtil = new.WeeklyUtil
		result.WeeklyResets = new.WeeklyResets
		result.HasOAuthData = true
	}

	if new.HasCostData {
		result.DailyCost = new.DailyCost
		result.HasCostData = true
	}

	if new.HasStatsData {
		result.MessageCount = new.MessageCount
		result.SessionCount = new.SessionCount
		result.HasStatsData = true
	}

	result.LastUpdated = new.LastUpdated
	result.Errors = new.Errors
	return result
}
