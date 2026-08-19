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

// truncateText clamps s to at most max visible runes, appending an
// ellipsis when it had to cut anything. Used anywhere a single line of
// dynamic content (service names, field values, preset descriptions) is
// placed inside a fixed-width box: without this, one long value silently
// grows the rendered line past the pane's border, which is what breaks
// alignment on resize or with real-world values.
func truncateText(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return string(r[:1])
	}
	return string(r[:max-1]) + "…"
}
