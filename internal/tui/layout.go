package tui

import "github.com/charmbracelet/lipgloss"

// joinHorizontal lays out the three rendered panes side by side, aligned
// to the top, using lipgloss.JoinHorizontal as required by the layout spec.
func joinHorizontal(left, center, right string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left, center, right)
}

// lipglossHeight returns the rendered height (in terminal rows) of a
// styled string, accounting for wrapped/multi-line content.
func lipglossHeight(s string) int {
	return lipgloss.Height(s)
}
