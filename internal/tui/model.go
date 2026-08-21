// Package tui implements the komposer terminal UI: a 3-pane Bubbletea
// application (services list, service config form, live YAML preview)
// built on top of the pkg/composer domain model.
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	saveAsDialog     saveAsDialog

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
	yamlViewport  viewport.Model
	viewportReady bool

	// Phase 4: save state
	lastSave saveResult
	// dirty tracks whether the in-memory config has changes that
	// haven't been written to disk since the last successful save.
	// Drives whether 'q' quits immediately or offers the save dialog
	// first.
	dirty bool

	quitting bool
}

// New returns a freshly initialized komposer Model.
func New() Model {
	return Model{
		config: composer.NewComposeConfig(),
		focus:  paneLeft,
	}
}

// Init hides the terminal's real (hardware) cursor for the program's
// lifetime. Bubble Tea does this automatically in the normal case, but
// there are terminals/console hosts where the hide sequence doesn't
// reliably stick — leaving a distracting blinking caret sitting
// somewhere on screen, separate from the styled cursor each focused
// textinput/textarea draws itself. Sending this explicitly costs
// nothing and is the documented, sanctioned way to force it.
func (m Model) Init() tea.Cmd {
	return tea.HideCursor
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case saveResult:
		m.lastSave = msg
		if msg.err == nil {
			m.dirty = false
			m.currentMode = modeSaved
			if msg.quitAfterSave {
				m.quitting = true
				return m, tea.Quit
			}
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return clearSaveBannerMsg{}
			})
		}
		// A failed save always returns to the normal view, even if it
		// was triggered while trying to quit — losing unsaved work
		// because of e.g. a permissions error would be worse than
		// just staying open so the user can see the error and retry.
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
		headerHeight := lipglossHeight(paneTitleStyle.Render("Preview: docker-compose.yml")) + 3
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

	case tea.MouseMsg:
		// Mouse wheel scrolling. Only wired up in normal mode (dialogs
		// don't scroll with the wheel — their content is either static
		// or the picker's own Up/Down windowing handles it) and only
		// for whichever pane currently has focus, matching how
		// keyboard scrolling already works.
		if m.currentMode != modeNormal {
			return m, nil
		}
		var cmd tea.Cmd
		switch m.focus {
		case paneRight:
			if m.viewportReady {
				m.yamlViewport, cmd = m.yamlViewport.Update(msg)
			}
		case paneLeft:
			if len(m.config.Services) > 0 && msg.Action == tea.MouseActionPress {
				switch msg.Button {
				case tea.MouseButtonWheelUp:
					m.selected = (m.selected - 1 + len(m.config.Services)) % len(m.config.Services)
				case tea.MouseButtonWheelDown:
					m.selected = (m.selected + 1) % len(m.config.Services)
				}
			}
		}
		return m, cmd

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
		case modeSaveAs:
			return m.updateSaveAs(msg)
		}

		// Normal mode keys
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "q":
			// Previously this quit immediately every time, with no
			// warning — any unsaved edits were silently lost. Now,
			// if there's nothing unsaved, quitting still needs no
			// ceremony; otherwise the same save dialog Ctrl+S uses
			// opens, with an explicit way to quit without saving.
			if !m.dirty {
				m.quitting = true
				return m, tea.Quit
			}
			m.currentMode = modeSaveAs
			m.saveAsDialog = newSaveAsDialog(true)
			return m, nil

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
			// Previously this wrote straight to a hardcoded
			// "docker-compose.yml" with no confirmation, and the only
			// feedback was a banner that appeared for two seconds —
			// unclear what had actually happened or where the file
			// went. Now it opens an explicit dialog with the target
			// path, prefilled with the same default, so saving is a
			// deliberate, visible action instead of a surprise.
			m.currentMode = modeSaveAs
			m.saveAsDialog = newSaveAsDialog(false)
			return m, nil

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

// updateSaveAs handles input in the "save to disk" dialog, shown by
// Ctrl+S and by 'q' when there are unsaved changes.
func (m Model) updateSaveAs(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// A file already exists at the chosen path — Enter here has moved
	// to a distinct yes/no confirmation instead of writing immediately,
	// same shape as the existing delete-service confirmation. Without
	// this, retyping the default path (or just hitting Enter again out
	// of habit) would silently clobber an existing docker-compose.yml.
	if m.saveAsDialog.confirmingOverwrite {
		switch msg.String() {
		case "y", "Y", "enter":
			quitAfterSave := m.saveAsDialog.quitAfterSave
			path := m.saveAsDialog.pendingPath
			m.currentMode = modeSaving
			return m, m.saveFileAs(path, quitAfterSave)
		case "n", "N":
			// Back to editing the path, not back to the main screen —
			// the most likely next step is picking a different name,
			// not abandoning the save entirely.
			m.saveAsDialog.confirmingOverwrite = false
			return m, nil
		case "esc":
			m.currentMode = modeNormal
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		// Cancel and go back — this never quits, even if the dialog
		// was opened via 'q': backing out of the save prompt should
		// mean "let me keep working", not "discard my changes".
		m.currentMode = modeNormal
		return m, nil

	case "enter":
		path := strings.TrimSpace(m.saveAsDialog.pathInput.Value())
		if path == "" {
			path = "docker-compose.yml"
		}
		if _, err := os.Stat(path); err == nil {
			m.saveAsDialog.confirmingOverwrite = true
			m.saveAsDialog.pendingPath = path
			return m, nil
		}
		quitAfterSave := m.saveAsDialog.quitAfterSave
		m.currentMode = modeSaving
		return m, m.saveFileAs(path, quitAfterSave)

	case "q":
		// Only treated as "quit without saving" when this dialog was
		// opened because of an in-progress quit. Otherwise 'q' is just
		// a character someone might type while editing the path.
		if m.saveAsDialog.quitAfterSave {
			m.quitting = true
			return m, tea.Quit
		}
	}

	m.saveAsDialog.pathInput, cmd = m.saveAsDialog.pathInput.Update(msg)
	return m, cmd
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
			m.dirty = true
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
		m.dirty = true
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
// Field navigation is arrow-key first, matching how the rest of the
// app already navigates (the service list, presets, everything else)
// — Tab/Shift+Tab still work as a direct alternative, but aren't
// required. Up/Down are context-sensitive: inside a multi-line field
// (Ports/Environment/Volumes) they move the cursor between the lines
// you've typed, same as any normal text editor; only at the top or
// bottom edge of the field's content do they cross over to the
// previous/next field, exactly like tabbing out of a form field once
// there's nowhere further to go inside it. Single-line fields
// (Image/Build/Restart) have no internal lines to move between, so
// Up/Down always switch fields there.
//
// Enter is forwarded to the focused field: on a list field that
// inserts a newline (textarea's native behavior — nothing to do here),
// on a single-line field it's a no-op, same as it's always been.
//
// Both Esc and Ctrl+S save the form to the service and return to
// normal mode. They're intentionally the same action under two keys —
// Ctrl+S for the explicit "I'm saving" muscle memory, Esc as the
// general "I'm done here" key used by every other dialog in the app.
// Neither one writes to disk; that's a separate, deliberate action via
// the main screen's Ctrl+S (see updateSaveAs).
func (m Model) updateEditField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "esc", "ctrl+s":
		m.saveFormToService()
		m.currentMode = modeNormal
		return m, nil

	case "tab":
		m.nextFormField()
		return m, nil

	case "shift+tab":
		m.prevFormField()
		return m, nil

	case "up":
		f := formField(m.focusedFormField)
		if !isListField(f) || m.formAreas[f].Line() == 0 {
			m.prevFormField()
			return m, nil
		}
		// Cursor isn't on the field's first line yet — let it move up
		// within the field instead of switching fields. Falls through
		// to the generic forwarding below.

	case "down":
		f := formField(m.focusedFormField)
		if !isListField(f) || m.formAreas[f].Line() >= m.formAreas[f].LineCount()-1 {
			m.nextFormField()
			return m, nil
		}
		// Cursor isn't on the field's last line yet — let it move down
		// within the field instead of switching fields.
	}

	f := formField(m.focusedFormField)
	if isListField(f) {
		if int(f) < len(m.formAreas) {
			m.formAreas[f], cmd = m.formAreas[f].Update(msg)
			syncListFieldHeight(&m.formAreas[f])
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

	var warnings []string
	for _, w := range result.Warnings {
		warnings = append(warnings, w.Error())
	}

	m.validationDialog = validationDialog{
		errors:   errors,
		warnings: warnings,
		scroll:   0,
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
	// The working copy now differs from whatever's on disk at the
	// default save path until it's explicitly saved, same as any other
	// change — this isn't itself the file at "docker-compose.yml".
	m.dirty = true
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
	case modeSaveAs:
		return m.renderSaveAsDialog()
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
			Render("[OK] Saved " + m.lastSave.filename)
		mainView = header + "\n" + banner + "\n" + body + "\n" + helpBar
	} else if m.lastSave.err != nil {
		banner := lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(colorDanger).
			Padding(0, 1).
			Bold(true).
			Render("[ERROR] " + m.lastSave.err.Error())
		mainView = header + "\n" + banner + "\n" + body + "\n" + helpBar
	}

	return mainView
}

// renderHeader renders the full-width branding bar at the top of the
// screen. Width is clamped to exactly m.width (accounting for the
// style's own border and padding) so the frame never overshoots or
// undershoots the terminal on resize.
//
// The title text is routed through truncateText before Width/MaxWidth
// are applied: MaxWidth alone only clips raw bytes at a hard boundary,
// it doesn't wrap or shorten gracefully, so on a narrow terminal part
// of the title used to vanish mid-word instead of ending in "…".
// truncateText does the graceful shortening; MaxWidth stays only as
// the same backstop used everywhere else.
//
// The title is now plain ASCII — no anchor emoji, no other symbol
// outside the base ASCII range. That's specifically because rendering
// consistency was reported to vary across terminals/fonts (PowerShell
// fine, others drifting): a glyph outside a font's normal coverage gets
// silently substituted from a fallback font by the terminal, and that
// substitute's rendered cell width often doesn't match the terminal's
// fixed grid — which desyncs every column calculation after it on that
// line. Plain ASCII has no such ambiguity in any monospace font.
func (m Model) renderHeader() string {
	// Border (1 col each side) + Padding(0, 2) (2 cols each side) = 6
	// columns of overhead before content width.
	w := m.width - 6
	if w < 1 {
		w = 1
	}
	title := truncateText("komposer - Docker Compose Builder", w)
	return headerStyle.Width(w).MaxWidth(m.width).Render(title)
}

func (m Model) renderHelpBar() string {
	var help string
	switch m.currentMode {
	case modeAddService:
		help = "enter: confirm • esc: cancel"
	case modeConfirmDelete:
		help = "y: delete • n: cancel"
	case modeEditField:
		help = "↑↓: move / switch field • enter: newline in list fields • tab: next field • ctrl+s/esc: save & close"
	case modePresetPicker:
		if m.presetPicker.stage == 0 {
			help = "↑↓: navigate • ←→: switch tab • enter: select • esc: cancel"
		} else {
			help = "enter: confirm • esc: back"
		}
	case modeValidation:
		help = "esc: close"
	case modeImport:
		help = "enter: import • esc: cancel"
	case modeSaveAs:
		if m.saveAsDialog.confirmingOverwrite {
			help = "y: overwrite • n: different path • esc: cancel"
		} else if m.saveAsDialog.quitAfterSave {
			help = "enter: save & quit • q: quit without saving • esc: cancel"
		} else {
			help = "enter: save • esc: cancel"
		}
	default:
		if m.focus == paneLeft {
			help = "↑↓: navigate • ←→: switch pane • a: add • d: delete • ctrl+p: presets • ctrl+o: import • ctrl+v: validate • ctrl+s: save • q: quit"
		} else if m.focus == paneCenter {
			help = "←→: switch pane • enter: edit • ctrl+p: presets • ctrl+o: import • ctrl+v: validate • ctrl+s: save • q: quit"
		} else {
			help = "↑↓: scroll • ←→: switch pane • ctrl+p: presets • ctrl+o: import • ctrl+v: validate • ctrl+s: save • q: quit"
		}
	}
	// helpStyle previously had no Width set at all, so this line — up to
	// ~130 chars in the default case — was rendered with zero width
	// constraint. lipgloss only word-wraps when Width is set (see
	// renderServiceForm); without it, a line wider than the terminal
	// just gets whatever the physical terminal does with an overlong
	// line under bubbletea's absolute cursor positioning, which is not
	// a clean wrap — parts of it effectively vanish rather than moving
	// to a second line. Width here makes it wrap like every other piece
	// of text in the app, and the two rows it can now take are already
	// accounted for below since contentHeight always calls
	// lipglossHeight(helpBar) on the real rendered result.
	w := m.width - 2 // helpStyle has Padding(0, 1) => 1 col each side
	if w < 1 {
		w = 1
	}
	return helpStyle.Width(w).Render(help)
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

// paneHeader renders a pane's title followed by a full-width divider
// line, visually separating the title from the body like a table
// header row. All three panes go through this so the title is always
// the first line of pane content and never gets mistaken for part of
// the scrollable body.
func paneHeader(title string, width int) string {
	divider := lipgloss.NewStyle().
		Foreground(colorMuted).
		Render(strings.Repeat("─", width))
	return paneTitleStyle.Render(title) + "\n" + divider
}

func (m Model) renderLeftPane(width, height int) string {
	var body strings.Builder
	names := m.config.ServiceNames()
	if len(names) == 0 {
		body.WriteString(helpStyle.Render("(none yet - press 'a')"))
	} else {
		// The list used to render every service unconditionally and
		// rely on clipping to hide whatever didn't fit — which meant
		// that with enough services, arrowing down past the visible
		// rows moved the selection to an item that had already been
		// clipped away, with nothing on screen to show it had even
		// moved. This windows the list around m.selected instead, the
		// same approach used by the preset/stack picker, so the
		// selected service is always visible while scrolling with
		// Up/Down.
		//
		// bodyRows leaves 2 lines for paneHeader (title + divider)
		// above, matching what content actually has room for once
		// `height` content rows are allotted to the whole pane.
		bodyRows := height - 2
		if bodyRows < 1 {
			bodyRows = 1
		}

		// When everything fits, show it all with no markers. When it
		// doesn't, reserve 2 rows for "more above"/"more below"
		// markers up front — simpler than computing exactly which of
		// the two will actually show for a given offset, at the cost
		// of one row of slack at the very top/bottom of the list.
		visible := bodyRows
		scrolling := len(names) > bodyRows
		if scrolling {
			visible = bodyRows - 2
			if visible < 1 {
				visible = 1
			}
		}
		if visible > len(names) {
			visible = len(names)
		}

		offset := 0
		if len(names) > visible {
			offset = m.selected - visible/2
			if offset < 0 {
				offset = 0
			}
			if offset > len(names)-visible {
				offset = len(names) - visible
			}
		}

		if offset > 0 {
			body.WriteString(helpStyle.Render(fmt.Sprintf("^ %d more above", offset)) + "\n")
		}

		for i := offset; i < offset+visible; i++ {
			cursor := "  "
			if i == m.selected {
				cursor = "> "
			}
			body.WriteString(cursor + truncateText(names[i], width-2) + "\n")
		}

		if remaining := len(names) - (offset + visible); remaining > 0 {
			body.WriteString(helpStyle.Render(fmt.Sprintf("v %d more below", remaining)))
		}
	}

	content := paneHeader("Services", width) + "\n" + body.String()

	// MaxWidth/MaxHeight are a deliberate second line of defense on top
	// of truncateText and paneHeader above, not a replacement for them:
	// those keep content from overflowing in the first place (correct,
	// ANSI-safe), Max* are a hard backstop in case something new is
	// added later and forgets to size itself correctly. Together they
	// don't conflict — Width/Height set the box's target size, Max*
	// caps it if content still overflows.
	//
	// MaxHeight specifically fixes titles disappearing on resize: Height()
	// alone in lipgloss only pads content that's SHORTER than it — it
	// never clips content that's TALLER. Once a service list (or a form,
	// or the YAML preview) grew past the pane's allotted rows, the pane
	// simply rendered taller than the terminal, and since the title is
	// the *first* line of that now-too-tall block, it's the title that
	// scrolled off the top and out of view — not the overflowing content
	// at the bottom, which stayed on screen. MaxHeight makes the box a
	// hard ceiling, so the title row is always preserved and only the
	// tail of long content is ever clipped.
	//
	// All three panes cap at the same height+2 (content height + border
	// overhead) so their bottom borders always land on the same row —
	// see clipLines' doc comment for why the actual clipping happens on
	// the raw content beforehand rather than relying on MaxHeight alone.
	content = clipLines(content, height)
	return paneStyle(m.focus == paneLeft).
		Width(width).
		MaxWidth(width + 4).
		Height(height).
		MaxHeight(height + 2).
		Render(content)
}

func (m Model) renderCenterPane(width, height int) string {
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

	content := paneHeader("Service Config", width) + "\n" + body

	// See clipLines' doc comment (layout.go) for why content is clipped
	// here instead of relying on MaxHeight alone to cap an overflowing
	// pane — MaxHeight by itself chops the bottom border off along with
	// the excess content once a style has a border.
	content = clipLines(content, height)
	return paneStyle(m.focus == paneCenter).
		Width(width).
		MaxWidth(width + 4).
		Height(height).
		MaxHeight(height + 2).
		Render(content)
}

func (m Model) renderRightPane(width, height int) string {
	yamlBytes, err := m.config.ExportYAML()
	body := string(yamlBytes)
	if err != nil {
		body = "error rendering yaml: " + err.Error()
	}
	if strings.TrimSpace(body) == "" || strings.TrimSpace(body) == "services: {}" {
		body = helpStyle.Render("(empty - add a service to see YAML)")
	}

	header := paneHeader("Preview: docker-compose.yml", width)

	// Use viewport for scrollable content when ready
	var content string
	if m.viewportReady {
		m.yamlViewport.SetContent(body)
		viewportContent := m.yamlViewport.View()
		content = header + "\n" + viewportContent
	} else {
		content = header + "\n" + body
	}

	// See clipLines' doc comment (layout.go) for why content is clipped
	// here instead of relying on MaxHeight alone.
	content = clipLines(content, height)
	return paneStyle(m.focus == paneRight).
		Width(width).
		MaxWidth(width + 4).
		Height(height).
		MaxHeight(height + 2).
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
