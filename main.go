// Command komposer is a Docker Compose editor and checker: with no
// arguments it launches the terminal UI; `komposer check` validates a
// Compose file non-interactively, for scripts and CI.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/casablanque-code/komposer/internal/check"
	"github.com/casablanque-code/komposer/internal/tui"
)

// version is set at build time via -ldflags "-X main.version=...";
// release binaries get their tag (see .goreleaser.yaml), a `go build`
// or `go run` from source keeps the "dev" placeholder.
var version = "dev"

const usage = `komposer — Docker Compose editor and checker

Usage:
  komposer                  open the interactive editor
  komposer check [file]     validate a Compose file (komposer check --help)
  komposer --version        print the version
  komposer --help           show this help
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Println("komposer", version)
			return
		case "--help", "-h", "help":
			fmt.Print(usage)
			return
		case "check":
			os.Exit(check.Run(os.Args[2:], os.Stdout, os.Stderr, checkStyler()))
		}
	}

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

// checkStyler colors `komposer check` output. The renderer detects
// whether stdout is a color-capable terminal (and honors NO_COLOR), so
// piped or redirected output stays plain.
func checkStyler() check.Styler {
	r := lipgloss.NewRenderer(os.Stdout)
	style := func(s lipgloss.Style) func(string) string {
		return func(text string) string { return s.Render(text) }
	}
	return check.Styler{
		Error: style(r.NewStyle().Foreground(lipgloss.Color("9"))),
		Warn:  style(r.NewStyle().Foreground(lipgloss.Color("11"))),
		OK:    style(r.NewStyle().Foreground(lipgloss.Color("10"))),
		Dim:   style(r.NewStyle().Faint(true)),
		Bold:  style(r.NewStyle().Bold(true)),
	}
}
