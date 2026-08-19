// Package tui implements the komposer terminal UI: a 3-pane Bubbletea
// application (services list, service config form, live YAML preview)
// built on top of the pkg/composer domain model.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/casablanque-code/komposer/pkg/composer"
)

// pane identifies which of the three panes currently has focus.
type pane int

const (
	paneLeft pane = iota
	paneCenter
	paneRight
)

// Width distribution for the 3-pane layout, as fractions of total width.
const (
	leftWidthFrac   = 0.25
	centerWidthFrac = 0.45
	rightWidthFrac  = 0.30
)

// Model is the root Bubbletea model for komposer.
type Model struct {
	config *composer.ComposeConfig
	focus  pane

	width  int
	height int

	// selected index into config.Services for the left pane cursor.
	// Real list navigation (bubbles/list) lands in Phase 3; for now this
	// just tracks which service name is highlighted.
	selected int

	quitting bool
}

// New returns a freshly initialized komposer Model.
func New() Model {
	return Model{
		config: composer.NewComposeConfig(),
		focus:  paneLeft,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "tab":
			m.focus = nextPane(m.focus)
			return m, nil

		case "shift+tab":
			m.focus = prevPane(m.focus)
			return m, nil
		}
	}

	return m, nil
}

func nextPane(p pane) pane {
	switch p {
	case paneLeft:
		return paneCenter
	case paneCenter:
		return paneRight
	default:
		return paneLeft
	}
}

func prevPane(p pane) pane {
	switch p {
	case paneLeft:
		return paneRight
	case paneCenter:
		return paneLeft
	default:
		return paneCenter
	}
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		// First frame, before the initial WindowSizeMsg has arrived.
		return "initializing..."
	}

	helpBar := helpStyle.Render(
		"tab: switch pane • a: add service • d: delete • ctrl+p: presets • ctrl+s: save • q: quit",
	)
	contentHeight := m.height - lipglossHeight(helpBar) - 1
	if contentHeight < 1 {
		contentHeight = 1
	}

	leftW, centerW, rightW := paneWidths(m.width)

	left := m.renderLeftPane(leftW, contentHeight)
	center := m.renderCenterPane(centerW, contentHeight)
	right := m.renderRightPane(rightW, contentHeight)

	body := joinHorizontal(left, center, right)

	return body + "\n" + helpBar
}

// paneWidths computes the inner content width (excluding the 2-column
// border/padding overhead lipgloss adds per pane) for each of the three
// panes given the total terminal width, following the 25/45/30 split from
// the spec.
func paneWidths(total int) (left, center, right int) {
	const frameOverhead = 4 // border (2) + horizontal padding (2) per pane

	left = int(float64(total)*leftWidthFrac) - frameOverhead
	center = int(float64(total)*centerWidthFrac) - frameOverhead
	right = total - left - center - 3*frameOverhead
	// right absorbs rounding drift so the three panes always sum to `total`

	if left < 1 {
		left = 1
	}
	if center < 1 {
		center = 1
	}
	if right < 1 {
		right = 1
	}
	return left, center, right
}

func (m Model) renderLeftPane(width, height int) string {
	title := paneTitleStyle.Render("Services")

	var body strings.Builder
	names := m.config.ServiceNames()
	if len(names) == 0 {
		body.WriteString(helpStyle.Render("(none yet — press 'a')"))
	} else {
		for i, name := range names {
			cursor := "  "
			if i == m.selected {
				cursor = "> "
			}
			body.WriteString(cursor + name + "\n")
		}
	}

	content := title + "\n" + body.String()
	return paneStyle(m.focus == paneLeft).Width(width).Height(height).Render(content)
}

func (m Model) renderCenterPane(width, height int) string {
	title := paneTitleStyle.Render("Service Config")

	body := helpStyle.Render("Select or add a service to edit it here.")
	if len(m.config.Services) > 0 {
		entry := m.config.Services[clamp(m.selected, 0, len(m.config.Services)-1)]
		body = renderServiceForm(entry)
	}

	content := title + "\n" + body
	return paneStyle(m.focus == paneCenter).Width(width).Height(height).Render(content)
}

func (m Model) renderRightPane(width, height int) string {
	title := paneTitleStyle.Render("Preview: docker-compose.yml")

	yamlBytes, err := m.config.ExportYAML()
	body := string(yamlBytes)
	if err != nil {
		body = "error rendering yaml: " + err.Error()
	}
	if strings.TrimSpace(body) == "" || strings.TrimSpace(body) == "services: {}" {
		body = helpStyle.Render("(empty — add a service to see YAML)")
	}

	content := title + "\n" + body
	return paneStyle(m.focus == paneRight).Width(width).Height(height).Render(content)
}

// renderServiceForm is a placeholder field dump; Phase 3 replaces this with
// real bubbles/textinput-backed fields.
func renderServiceForm(entry composer.ServiceEntry) string {
	c := entry.Config
	lines := []string{
		"name:        " + entry.Name,
		"image:       " + c.Image,
		"build:       " + c.Build,
		"ports:       " + strings.Join(c.Ports, ", "),
		"environment: " + strings.Join(c.Environment, ", "),
		"volumes:     " + strings.Join(c.Volumes, ", "),
	}
	return strings.Join(lines, "\n")
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
