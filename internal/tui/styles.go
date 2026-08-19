package tui

import "github.com/charmbracelet/lipgloss"

// Color palette. Kept centralized so later phases (presets modal, banners)
// can reuse the same accents instead of hardcoding hex values.
var (
	colorAccent    = lipgloss.Color("62")  // focused pane border / highlights
	colorMuted     = lipgloss.Color("240") // unfocused pane border
	colorSubtle    = lipgloss.Color("246") // secondary text
	colorTitle     = lipgloss.Color("230") // pane titles
	colorSuccess   = lipgloss.Color("42")  // save banner
	colorDanger    = lipgloss.Color("204") // delete/error banner
)

var (
	paneTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTitle).
			Padding(0, 1)

	basePaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1)

	focusedPaneStyle = basePaneStyle.
				BorderForeground(colorAccent)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Padding(0, 1)
)

// paneStyle returns the border style for a pane depending on whether it
// currently has focus.
func paneStyle(focused bool) lipgloss.Style {
	if focused {
		return focusedPaneStyle
	}
	return basePaneStyle
}
