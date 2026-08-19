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
	p := tea.NewProgram(tui.New(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "komposer: error:", err)
		os.Exit(1)
	}
}
