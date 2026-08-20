// Package tui implements the komposer terminal UI: a 3-pane Bubbletea
// application (services list, service config form, live YAML preview)
// built on top of the pkg/composer domain model.
package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
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
	validationDialog validationDialog
	importDialog     importDialog

	// Phase 3: editable form fields for center pane.
	// Image/Build/Restart are single-line values, so they stay as
	// textinput. Ports/Environment/Volumes are lists, so they're edited
	// as textarea (one item per line) — see isListField in form.go.
	// Both slices are always len(numFormFields); only the entries that
	// match a field's kind are actually used, so index i in either
	// slice always corresponds to formField(i).
	formInputs       []textinput.Model
	formAreas        []textarea.Model
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

		// Keep the YAML preview viewport's internal width/height in sync
		// with the current terminal size on EVERY WindowSizeMsg, not just
		// the first. Previously this block was gated behind
		// `if !m.viewportReady`, so after the initial resize the
		// viewport's content kept rendering at its original width/height
		// forever. When the terminal was later shrunk, the pane's border
		// (recomputed correctly via paneWidths) became narrower than the
		// viewport's stale, wider content — the content spilled past the
		// right border instead of being clipped/rewrapped to it. That's
		// the direct cause of the right border "disappearing" and the
		// `|` characters drifting on resize.
		_, _, rightW := paneWidths(m.width)
		headerHeight := lipglossHeight(paneTitleStyle.Render("Preview: docker-compose.yml")) + 2
		viewportContentHeight := m.height - headerHeight - 2
		if viewportContentHeight < 1 {
			viewportContentHeight = 1
		}
		if !m.viewportReady {
			m.yamlViewport = viewport.New(rightW, viewportContentHeight)
			m.viewportReady = true
		} else {
			m.yamlViewport.Width = rightW
			m.yamlViewport.Height = viewportContentHeight
		}

		// Keep the editable form's input widths in sync with the current
		// terminal size. Previously these were computed once in
		// initFormInputs() and never revisited, so resizing the terminal
		// while editing left the fields sized for the old width — a
		// direct cause of the border/content misalignment on resize.
		if m.currentMode == modeEditField && len(m.formInputs) > 0 {
			_, centerW, _ := paneWidths(m.width)
			fieldWidth := centerW - 14 // 12 for label + 2 for spacing
			if fieldWidth < 20 {
				fieldWidth = 20
			}
			for i := range m.formInputs {
				m.formInputs[i].Width = fieldWidth
			}
			for i := range m.formAreas {
				m.formAreas[i].SetWidth(fieldWidth)
			}
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
		case modeValidation:
			return m.updateValidation(msg)
		case modeImport:
			return m.updateImport(msg)
		}

		// Normal mode keys
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "left", "h":
			m.focus = prevPane(m.focus)
			return m, nil

		case "right", "l":
			m.focus = nextPane(m.focus)
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

		case "ctrl+v":
			m.showValidation()
			return m, nil

		case "ctrl+o":
			m.currentMode = modeImport
			m.importDialog = newImportDialog()
			return m, nil

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
//
// Enter used to save the whole form and exit edit mode — there was no
// way to type a newline, so list fields (ports/environment/volumes)
// could only ever be one comma-separated line. Now:
//   - Esc saves and exits, same as before.
//   - Ctrl+S saves (both to the in-memory service and to disk) without
//     leaving edit mode, so you can keep working.
//   - Enter is forwarded to the focused field: on a list field
//     (textarea) that inserts a newline, so you can add rows; on a
//     single-line field (textinput) it's a no-op, same as it always
//     was for those.
//   - Field navigation moved from Up/Down to Tab/Shift+Tab, because
//     Up/Down are now needed inside the list fields to move the cursor
//     between the lines you've typed.
func (m Model) updateEditField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "esc":
		m.saveFormToService()
		m.currentMode = modeNormal
		return m, nil

	case "ctrl+s":
		m.saveFormToService()
		return m, m.saveFile()

	case "tab":
		m.nextFormField()
		return m, nil

	case "shift+tab":
		m.prevFormField()
		return m, nil
	}

	f := formField(m.focusedFormField)
	if isListField(f) {
		if int(f) < len(m.formAreas) {
			m.formAreas[f], cmd = m.formAreas[f].Update(msg)
			m.saveFormToService()
		}
	} else if int(f) < len(m.formInputs) {
		m.formInputs[f], cmd = m.formInputs[f].Update(msg)
		m.saveFormToService()
	}

	return m, cmd
}

// updateValidation handles input in the validation dialog.
func (m Model) updateValidation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter":
		m.currentMode = modeNormal
		return m, nil
	}
	return m, nil
}

// updateImport handles input in the import dialog.
func (m Model) updateImport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "esc":
		m.currentMode = modeNormal
		return m, nil

	case "enter":
		path := strings.TrimSpace(m.importDialog.pathInput.Value())
		if path != "" {
			if err := m.importFile(path); err != nil {
				m.lastSave = saveResult{filename: path, err: err}
			} else {
				m.lastSave = saveResult{filename: path, err: nil}
				m.selected = 0
			}
		}
		m.currentMode = modeNormal
		return m, nil
	}

	m.importDialog.pathInput, cmd = m.importDialog.pathInput.Update(msg)
	return m, cmd
}

// showValidation runs validation and displays results in a dialog.
func (m *Model) showValidation() {
	result := m.config.Validate()

	var errors []string
	for _, err := range result.Errors {
		errors = append(errors, err.Error())
	}

	m.validationDialog = validationDialog{
		errors: errors,
		scroll: 0,
	}
	m.currentMode = modeValidation
}

// importFile loads a docker-compose.yml file into the current config.
func (m *Model) importFile(path string) error {
	imported, err := composer.ImportYAML(path)
	if err != nil {
		return err
	}
	m.config = imported
	return nil
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

	// Render dialogs in full-screen overlay modes
	switch m.currentMode {
	case modeAddService:
		return m.renderAddServiceDialog()
	case modeConfirmDelete:
		return m.renderConfirmDeleteDialog()
	case modePresetPicker:
		return m.renderPresetPickerDialog()
	case modeValidation:
		return m.renderValidationDialog()
	case modeImport:
		return m.renderImportDialog()
	}

	// Normal view rendering
	header := m.renderHeader()
	helpBar := m.renderHelpBar()

	// The save/error banner (if shown) adds one extra row above the main
	// view. Reserve that row up front so toggling the banner never shifts
	// the pane layout by a line — this was previously computed after
	// contentHeight, causing the whole UI to visibly jump on every save.
	bannerLines := 0
	if m.currentMode == modeSaved || m.lastSave.err != nil {
		bannerLines = 1
	}

	// contentHeight is the exact number of rows available to the 3 panes:
	// total terminal rows, minus the header, help bar, and banner rows.
	contentHeight := m.height - lipglossHeight(header) - lipglossHeight(helpBar) - bannerLines
	if contentHeight < 1 {
		contentHeight = 1
	}

	leftW, centerW, rightW := paneWidths(m.width)

	left := m.renderLeftPane(leftW, contentHeight)
	center := m.renderCenterPane(centerW, contentHeight)
	right := m.renderRightPane(rightW, contentHeight)

	body := joinHorizontal(left, center, right)
	mainView := header + "\n" + body + "\n" + helpBar

	if m.currentMode == modeSaved && m.lastSave.err == nil {
		banner := lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(colorSuccess).
			Padding(0, 1).
			Bold(true).
			Render("✓ Saved " + m.lastSave.filename)
		mainView = header + "\n" + banner + "\n" + body + "\n" + helpBar
	} else if m.lastSave.err != nil {
		banner := lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(colorDanger).
			Padding(0, 1).
			Bold(true).
			Render("✗ Error: " + m.lastSave.err.Error())
		mainView = header + "\n" + banner + "\n" + body + "\n" + helpBar
	}

	return mainView
}

// renderHeader renders the full-width branding bar at the top of the
// screen. Width is clamped to exactly m.width (minus the style's own
// horizontal padding) so the background color never overshoots or
// undershoots the terminal on resize.
func (m Model) renderHeader() string {
	w := m.width - 4 // headerStyle has Padding(0, 2) => 2 cols each side
	if w < 1 {
		w = 1
	}
	return headerStyle.Width(w).MaxWidth(m.width).Render("⚓ komposer — Docker Compose Builder")
}

func (m Model) renderHelpBar() string {
	var help string
	switch m.currentMode {
	case modeAddService:
		help = "enter: confirm • esc: cancel"
	case modeConfirmDelete:
		help = "y: delete • n: cancel"
	case modeEditField:
		help = "↑↓: next/prev field • enter/esc: save & close"
	case modePresetPicker:
		if m.presetPicker.stage == 0 {
			help = "↑↓: navigate • enter: select • esc: cancel"
		} else {
			help = "enter: confirm • esc: back"
		}
	case modeValidation:
		help = "esc: close"
	case modeImport:
		help = "enter: import • esc: cancel"
	default:
		if m.focus == paneLeft {
			help = "↑↓: navigate • ←→: switch pane • a: add • d: delete • ctrl+p: presets • ctrl+o: import • ctrl+v: validate • ctrl+s: save • q: quit"
		} else if m.focus == paneCenter {
			help = "←→: switch pane • enter: edit • ctrl+p: presets • ctrl+o: import • ctrl+v: validate • ctrl+s: save • q: quit"
		} else {
			help = "↑↓: scroll • ←→: switch pane • ctrl+p: presets • ctrl+o: import • ctrl+v: validate • ctrl+s: save • q: quit"
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

	// Calculate raw widths
	leftRaw := int(float64(total) * leftWidthFrac)
	centerRaw := int(float64(total) * centerWidthFrac)
	rightRaw := total - leftRaw - centerRaw

	// Subtract frame overhead
	left = leftRaw - frameOverhead
	center = centerRaw - frameOverhead
	right = rightRaw - frameOverhead

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
			body.WriteString(cursor + truncateText(name, width-2) + "\n")
		}
	}

	content := title + "\n" + body.String()

	// MaxWidth is a deliberate second line of defense on top of
	// truncateText above, not a replacement for it: truncateText keeps
	// content from overflowing in the first place (correct, ANSI-safe),
	// MaxWidth is a hard backstop in case something new is added later
	// and forgets to truncate. Together they don't conflict — Width sets
	// the box's target size, MaxWidth caps it if content still overflows.
	return paneStyle(m.focus == paneLeft).
		Width(width).
		MaxWidth(width + 4).
		Height(height).
		Render(content)
}

func (m Model) renderCenterPane(width, height int) string {
	title := paneTitleStyle.Render("Service Config")

	var body string
	if len(m.config.Services) == 0 {
		hint := "No services yet. Switch to the left pane and press 'a' to add one."
		if m.focus == paneLeft {
			hint = "Press 'a' to add a service."
		}
		body = helpStyle.Render(hint)
	} else {
		entry := m.config.Services[clamp(m.selected, 0, len(m.config.Services)-1)]
		if m.currentMode == modeEditField {
			body = m.renderEditableForm()
		} else {
			body = renderServiceForm(entry, width)
			if m.focus == paneCenter {
				hint := "\n\n" + helpStyle.Render("Press Enter or 'e' to edit")
				body += hint
			}
		}
	}

	content := title + "\n" + body

	return paneStyle(m.focus == paneCenter).
		Width(width).
		MaxWidth(width + 4).
		Height(height).
		Render(content)
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

	return paneStyle(m.focus == paneRight).
		Width(width).
		MaxWidth(width + 4).
		Height(height).
		Render(content)
}

// renderServiceForm renders the read-only field summary shown in the
// center pane outside of edit mode. Every line is wrapped to `width`
// instead of truncated: a long image name, port list, or env var string
// used to get cut off with "…" the moment the terminal got narrow, which
// silently hid part of the actual config. lipgloss wraps automatically
// when Width is set and content is longer than that, without dropping
// anything — it just uses more vertical space, which the pane's fixed
// Height() below will clip if it truly doesn't fit, but nothing is
// hidden behind an ellipsis anymore.
func renderServiceForm(entry composer.ServiceEntry, width int) string {
	c := entry.Config
	lines := []string{
		"name:        " + entry.Name,
		"image:       " + c.Image,
		"build:       " + c.Build,
		"ports:       " + strings.Join(c.Ports, ", "),
		"environment: " + strings.Join(c.Environment, ", "),
		"volumes:     " + strings.Join(c.Volumes, ", "),
	}
	wrapStyle := lipgloss.NewStyle().Width(width)
	for i, l := range lines {
		lines[i] = wrapStyle.Render(l)
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
