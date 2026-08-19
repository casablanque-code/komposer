// Package tui implements the komposer terminal UI: a 3-pane Bubbletea
// application (services list, service config form, live YAML preview)
// built on top of the pkg/composer domain model.
package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
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

	selected int

	// Phase 3: UI modes and dialogs
	currentMode      mode
	addDialog        addServiceDialog
	confirmDelete    confirmDeleteDialog
	presetPicker     presetPickerDialog

	// Phase 3: editable form fields for center pane
	formInputs       []textinput.Model
	focusedFormField int

	// Phase 3: scrollable YAML preview
	yamlViewport viewport.Model
	viewportReady bool

	// Phase 4: save state
	lastSave saveResult

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
	case saveResult:
		m.lastSave = msg
		if msg.err == nil {
			m.currentMode = modeSaved
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return clearSaveBannerMsg{}
			})
		}
		m.currentMode = modeNormal
		return m, nil

	case clearSaveBannerMsg:
		if m.currentMode == modeSaved {
			m.currentMode = modeNormal
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Initialize viewport for YAML preview on first WindowSizeMsg
		if !m.viewportReady {
			_, _, rightW := paneWidths(m.width)
			headerHeight := lipglossHeight(paneTitleStyle.Render("Preview: docker-compose.yml")) + 2
			contentHeight := m.height - headerHeight - 2
			if contentHeight < 1 {
				contentHeight = 1
			}
			m.yamlViewport = viewport.New(rightW, contentHeight)
			m.viewportReady = true
		}
		return m, nil

	case tea.KeyMsg:
		// Handle mode-specific keys first
		switch m.currentMode {
		case modeAddService:
			return m.updateAddService(msg)
		case modeConfirmDelete:
			return m.updateConfirmDelete(msg)
		case modeEditField:
			return m.updateEditField(msg)
		case modePresetPicker:
			return m.updatePresetPicker(msg)
		}

		// Normal mode keys
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

		case "a":
			if m.focus == paneLeft {
				m.currentMode = modeAddService
				m.addDialog = newAddServiceDialog()
			}
			return m, nil

		case "d":
			if m.focus == paneLeft && len(m.config.Services) > 0 {
				m.currentMode = modeConfirmDelete
				m.confirmDelete = confirmDeleteDialog{
					serviceName: m.config.Services[m.selected].Name,
				}
			}
			return m, nil

		case "ctrl+p":
			m.currentMode = modePresetPicker
			m.presetPicker = newPresetPickerDialog()
			return m, nil

		case "ctrl+s":
			return m, m.saveFile()

		case "e", "enter":
			if m.focus == paneCenter && len(m.config.Services) > 0 {
				m.currentMode = modeEditField
				m.initFormInputs()
			}
			return m, nil

		// Scroll YAML preview when right pane is focused
		case "up", "k":
			if m.focus == paneRight && m.viewportReady {
				m.yamlViewport.LineUp(1)
				return m, nil
			} else if m.focus == paneLeft && len(m.config.Services) > 0 {
				m.selected = (m.selected - 1 + len(m.config.Services)) % len(m.config.Services)
				return m, nil
			}
			return m, nil

		case "down", "j":
			if m.focus == paneRight && m.viewportReady {
				m.yamlViewport.LineDown(1)
				return m, nil
			} else if m.focus == paneLeft && len(m.config.Services) > 0 {
				m.selected = (m.selected + 1) % len(m.config.Services)
				return m, nil
			}
			return m, nil

		case "pgup":
			if m.focus == paneRight && m.viewportReady {
				m.yamlViewport.ViewUp()
				return m, nil
			}

		case "pgdown":
			if m.focus == paneRight && m.viewportReady {
				m.yamlViewport.ViewDown()
				return m, nil
			}
		}
	}

	return m, nil
}

// updateAddService handles input in the "add service" dialog.
func (m Model) updateAddService(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "esc":
		m.currentMode = modeNormal
		return m, nil

	case "enter":
		name := strings.TrimSpace(m.addDialog.nameInput.Value())
		if name != "" && m.config.GetService(name) == nil {
			m.config.AddService(name)
			m.selected = len(m.config.Services) - 1
		}
		m.currentMode = modeNormal
		return m, nil
	}

	m.addDialog.nameInput, cmd = m.addDialog.nameInput.Update(msg)
	return m, cmd
}

// updateConfirmDelete handles input in the delete confirmation dialog.
func (m Model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.config.RemoveService(m.confirmDelete.serviceName)
		if m.selected >= len(m.config.Services) && len(m.config.Services) > 0 {
			m.selected = len(m.config.Services) - 1
		}
		m.currentMode = modeNormal
		return m, nil

	case "n", "N", "esc":
		m.currentMode = modeNormal
		return m, nil
	}

	return m, nil
}

// updateEditField handles input in the form edit mode.
func (m Model) updateEditField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "esc":
		m.saveFormToService()
		m.currentMode = modeNormal
		return m, nil

	case "enter":
		m.saveFormToService()
		m.currentMode = modeNormal
		return m, nil

	case "tab":
		m.nextFormField()
		return m, nil

	case "shift+tab":
		m.prevFormField()
		return m, nil
	}

	// Update the focused input field
	if len(m.formInputs) > 0 && m.focusedFormField < len(m.formInputs) {
		m.formInputs[m.focusedFormField], cmd = m.formInputs[m.focusedFormField].Update(msg)
		// Sync changes to config in real-time for preview
		m.saveFormToService()
	}

	return m, cmd
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
		return "initializing..."
	}

	helpBar := m.renderHelpBar()
	contentHeight := m.height - lipglossHeight(helpBar) - 1
	if contentHeight < 1 {
		contentHeight = 1
	}

	leftW, centerW, rightW := paneWidths(m.width)

	left := m.renderLeftPane(leftW, contentHeight)
	center := m.renderCenterPane(centerW, contentHeight)
	right := m.renderRightPane(rightW, contentHeight)

	body := joinHorizontal(left, center, right)

	mainView := body + "\n" + helpBar

	if m.currentMode == modeSaved && m.lastSave.err == nil {
		banner := lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(colorSuccess).
			Padding(0, 2).
			Bold(true).
			Render("✓ Saved " + m.lastSave.filename)
		mainView = banner + "\n" + mainView
	} else if m.lastSave.err != nil {
		banner := lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(colorDanger).
			Padding(0, 2).
			Bold(true).
			Render("✗ Error: " + m.lastSave.err.Error())
		mainView = banner + "\n" + mainView
	}

	switch m.currentMode {
	case modeAddService:
		return mainView + "\n\n" + m.renderAddServiceDialog()
	case modeConfirmDelete:
		return mainView + "\n\n" + m.renderConfirmDeleteDialog()
	case modePresetPicker:
		return mainView + "\n\n" + m.renderPresetPickerDialog()
	}

	return mainView
}

func (m Model) renderHelpBar() string {
	var help string
	switch m.currentMode {
	case modeAddService:
		help = "enter: confirm • esc: cancel"
	case modeConfirmDelete:
		help = "y: delete • n: cancel"
	case modeEditField:
		help = "tab/shift+tab: next/prev field • enter/esc: save & close"
	case modePresetPicker:
		if m.presetPicker.stage == 0 {
			help = "↑↓: navigate • enter: select • esc: cancel"
		} else {
			help = "enter: confirm • esc: back"
		}
	default:
		if m.focus == paneLeft {
			help = "↑↓: navigate • a: add • d: delete • ctrl+p: presets • ctrl+s: save • tab: switch • q: quit"
		} else if m.focus == paneCenter {
			help = "enter/e: edit • ctrl+s: save • tab: switch • q: quit"
		} else {
			help = "↑↓: scroll • ctrl+s: save • tab: switch • q: quit"
		}
	}
	return helpStyle.Render(help)
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

	var body string
	if len(m.config.Services) == 0 {
		body = helpStyle.Render("Press 'a' to add a service")
	} else {
		entry := m.config.Services[clamp(m.selected, 0, len(m.config.Services)-1)]
		if m.currentMode == modeEditField {
			body = m.renderEditableForm()
		} else {
			body = renderServiceForm(entry)
			if m.focus == paneCenter {
				hint := "\n\n" + helpStyle.Render("Press Enter or 'e' to edit")
				body += hint
			}
		}
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

	// Use viewport for scrollable content when ready
	var content string
	if m.viewportReady {
		m.yamlViewport.SetContent(body)
		viewportContent := m.yamlViewport.View()
		content = title + "\n" + viewportContent
	} else {
		content = title + "\n" + body
	}

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
