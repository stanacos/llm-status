package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Braille characters for progress bar rendering.
// Each character represents a different fill level within a cell.
var brailleChars = []rune{'⣀', '⣏', '⣧', '⣷', '⣿'}

// renderBrailleBar renders a braille-character progress bar of the given width.
func renderBrailleBar(percent float64, width int) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	// Total "sub-cells" across the bar (4 levels per cell)
	totalUnits := width * 4
	filledUnits := int(percent / 100.0 * float64(totalUnits))

	filledColor := gradientColor(percent)
	filledStyle := lipgloss.NewStyle().Foreground(filledColor)
	emptyStyle := lipgloss.NewStyle().Foreground(colorComment)

	var bar strings.Builder
	unitsRemaining := filledUnits

	for i := 0; i < width; i++ {
		if unitsRemaining >= 4 {
			bar.WriteString(filledStyle.Render(string(brailleChars[4])))
			unitsRemaining -= 4
		} else if unitsRemaining > 0 {
			bar.WriteString(filledStyle.Render(string(brailleChars[unitsRemaining])))
			unitsRemaining = 0
		} else {
			bar.WriteString(emptyStyle.Render(string(brailleChars[0])))
		}
	}

	return bar.String()
}

// renderPanel wraps content in a rounded-border panel with a title injected into the top border.
func renderPanel(title string, content string, outerWidth int) string {
	innerWidth := outerWidth - 4 // 2 for border + 2 for padding

	styled := lipgloss.NewStyle().
		Width(innerWidth).
		Render(content)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorComment).
		Padding(0, 1).
		Width(outerWidth).
		Render(styled)

	return injectBorderTitle(box, title)
}

// injectBorderTitle replaces the top border line with one containing a colored title.
func injectBorderTitle(box string, title string) string {
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}

	totalWidth := lipgloss.Width(lines[0])
	if totalWidth < 6 {
		return box
	}

	bc := lipgloss.NewStyle().Foreground(colorComment)
	titleStyled := lipgloss.NewStyle().Foreground(colorPurple).Bold(true).Render(title)

	// Build: ╭── Title ─────────╮
	prefix := bc.Render("╭── ")
	suffix := bc.Render("╮")
	titleWithSpace := titleStyled + " "

	// Visible width consumed: 4 (╭── ) + title len + 1 (space) + 1 (╮)
	usedWidth := 4 + len(title) + 1 + 1
	dashCount := totalWidth - usedWidth
	if dashCount < 0 {
		dashCount = 0
	}

	lines[0] = prefix + titleWithSpace + bc.Render(strings.Repeat("─", dashCount)) + suffix
	return strings.Join(lines, "\n")
}

// renderMetricRow renders a label on the left and value on the right, spanning the given width.
func renderMetricRow(label string, value string, width int) string {
	labelStyled := lipgloss.NewStyle().Foreground(colorForeground).Render(label)
	valueStyled := lipgloss.NewStyle().Foreground(colorCyan).Render(value)

	labelWidth := lipgloss.Width(labelStyled)
	valueWidth := lipgloss.Width(valueStyled)
	gap := width - labelWidth - valueWidth
	if gap < 1 {
		gap = 1
	}

	return labelStyled + strings.Repeat(" ", gap) + valueStyled
}

// formatDuration converts a duration to a human-readable format.
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// formatPercent formats a float as a percentage string.
func formatPercent(pct float64) string {
	return fmt.Sprintf("%.0f%%", pct)
}

// formatCost formats a float as a USD cost string.
func formatCost(cost float64) string {
	return fmt.Sprintf("$%.2f", cost)
}

// formatInt formats an integer as a string.
func formatInt(n int) string {
	return fmt.Sprintf("%d", n)
}
