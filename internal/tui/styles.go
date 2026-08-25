package tui

import "github.com/charmbracelet/lipgloss"

// Color palette. Kept centralized so later phases (presets modal, banners)
// can reuse the same accents instead of hardcoding hex values.
var (
	colorAccent  = lipgloss.Color("62")  // focused pane border / highlights
	colorMuted   = lipgloss.Color("240") // unfocused pane border
	colorSubtle  = lipgloss.Color("246") // secondary text
	colorTitle   = lipgloss.Color("230") // pane titles
	colorSuccess = lipgloss.Color("42")  // save banner
	colorWarning = lipgloss.Color("214") // non-blocking validation warnings
	colorDanger  = lipgloss.Color("204") // delete/error banner
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

	// helpKeyStyle highlights the key-combo portion of each help bar
	// entry (e.g. "ctrl+s" in "ctrl+s: save") in the same accent used
	// for focus highlights elsewhere, so the keys read as distinct
	// actionable chips instead of blending into the surrounding
	// sentence-like gray text. helpDescStyle keeps the description
	// half in the existing muted gray — see styleHelpText.
	helpKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(colorSubtle)

	// headerStyle is the full-width branding bar at the very top of the
	// screen. Previously there was no top-level header at all — only
	// per-pane titles inside the borders — which made the app start
	// abruptly at row 0 with no breathing room.
	//
	// It now has its own rounded border matching the three panes below,
	// instead of a solid-color bar with no frame. The filled background
	// is dropped too — an unfilled bordered box with bold accent text is
	// the same visual language the three panes already use, so the
	// header reads as the top of one continuous frame instead of a
	// separate, disconnected bar sitting above it.
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(0, 2)
)

// paneStyle returns the border style for a pane depending on whether it
// currently has focus.
func paneStyle(focused bool) lipgloss.Style {
	if focused {
		return focusedPaneStyle
	}
	return basePaneStyle
}
