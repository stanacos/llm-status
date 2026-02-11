package main

import "github.com/charmbracelet/lipgloss"

// Dracula Pro color palette
var (
	colorBackground  = lipgloss.Color("#282A36")
	colorCurrentLine = lipgloss.Color("#44475A")
	colorForeground  = lipgloss.Color("#F8F8F2")
	colorComment     = lipgloss.Color("#6272A4")
	colorPurple      = lipgloss.Color("#BD93F9")
	colorCyan        = lipgloss.Color("#8BE9FD")
	colorGreen       = lipgloss.Color("#50FA7B")
	colorPink        = lipgloss.Color("#FF79C6")
	colorOrange      = lipgloss.Color("#FFB86C")
	colorRed         = lipgloss.Color("#FF5555")
	colorYellow      = lipgloss.Color("#F1FA8C")
)

// gradientColor returns the appropriate color based on usage percentage.
func gradientColor(percent float64) lipgloss.Color {
	switch {
	case percent < 40:
		return colorGreen
	case percent < 65:
		return colorYellow
	case percent < 85:
		return colorOrange
	default:
		return colorRed
	}
}
