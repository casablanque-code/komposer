// Command komposer launches the terminal UI for building a
// docker-compose.yml file interactively.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/casablanque-code/komposer/internal/tui"
)

func main() {
	// WithMouseCellMotion enables mouse wheel scrolling — without it,
	// the terminal never sends mouse events to the program at all, so
	// the YAML preview's viewport (which already handles wheel events
	// internally) and the services list silently ignored the wheel
	// no matter what.
	p := tea.NewProgram(tui.New(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "komposer: error:", err)
		os.Exit(1)
	}
}
