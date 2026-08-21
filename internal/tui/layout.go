package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

// clipLines returns at most maxLines lines of s, dropping any beyond
// that from the bottom.
//
// This exists because lipgloss's own MaxHeight doesn't do what it
// sounds like it does when a style also has a border: border is
// applied to the string BEFORE MaxHeight truncates it, and MaxHeight
// just keeps the first N lines of that already-bordered block. When
// content overflows past the pane's target height, that means the
// real bottom border line — which is the very last line of a
// longer-than-expected string — gets cut off along with the excess
// content, instead of the excess content being cut and the border
// staying put. The pane then renders with no bottom edge at all.
//
// Clipping the raw content to `height` lines ourselves, before it ever
// reaches paneStyle().Render(), sidesteps this: the border is always
// added to a string that's already within budget, so it's always the
// true last line of the render. MaxHeight is kept as a backstop set to
// height+2 (border overhead) — with content pre-clipped, it should
// never actually need to trigger.
func clipLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n")
}
