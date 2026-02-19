package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestInjectBorderTitleKeepsVisibleWidthWithWideTitle(t *testing.T) {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorComment).
		Padding(0, 1).
		Width(30).
		Render("content")

	got := injectBorderTitle(box, "毎日")

	originalTop := strings.Split(box, "\n")[0]
	updatedTop := strings.Split(got, "\n")[0]

	if gotWidth, wantWidth := lipgloss.Width(updatedTop), lipgloss.Width(originalTop); gotWidth != wantWidth {
		t.Fatalf("top border width mismatch: got %d want %d", gotWidth, wantWidth)
	}
	if !strings.Contains(updatedTop, "毎日") {
		t.Fatalf("updated title line missing title: %q", updatedTop)
	}
}
