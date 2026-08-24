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

	// editSnapshot captures the selected service's config as it was
	// the moment edit mode was entered. Esc compares the form's
	// current values against this to decide whether there's actually
	// anything to discard — and, if the user confirms discarding,
	// what to revert the live config back to. Needed because every
	// keystroke while editing is already applied straight to
	// m.config (see saveFormToService) so the YAML preview can update
	// live; there's no separate "unsaved draft" to just walk away
	// from, so discarding means actively restoring these values.
	editSnapshot          composer.ServiceConfig
	confirmingDiscardEdit bool

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

// previewBody returns the YAML preview's text content: the exported
// compose YAML, or a placeholder message if it's empty or failed to
// export. This is the single source for what the viewport should be
// showing — used both to keep the persisted viewport in sync (see
// refreshViewportContent) and for direct rendering in renderRightPane.
func (m Model) previewBody() string {
	yamlBytes, err := m.config.ExportYAML()
	// YAML marshaling conventionally ends output with a trailing
	// newline. Left in, viewport.SetContent's internal
	// strings.Split(s, "\n") turns that into one extra, entirely blank
	// trailing line — silently eating a row of the viewport's actual
	// content budget for nothing.
	body := strings.TrimRight(string(yamlBytes), "\n")
	if err != nil {
		body = "error rendering yaml: " + err.Error()
	}
	if strings.TrimSpace(body) == "" || strings.TrimSpace(body) == "services: {}" {
		body = helpStyle.Render("(empty - add a service to see YAML)")
	}
	return body
}

// refreshViewportContent keeps the persisted YAML viewport's content in
// sync with the current config. This has to run from Update(), not
// from View()/renderRightPane: View() has a value receiver, so a
// SetContent() call made there only ever mutates a throwaway copy of
// the model for that one render — it's never persisted back into the
// program's actual running state, which is whatever Update() returns.
//
// That mattered a lot more than it sounds like it should: bubbles'
// viewport.ScrollDown/ScrollUp both bail out immediately whenever the
// viewport's internal line count is zero (`len(m.lines) == 0`) — and a
// viewport that's only ever had SetContent called on a throwaway copy
// has an internal line count of zero in the copy Update() actually
// operates on. So LineUp/LineDown were reaching the viewport correctly
// and calling the right methods on every keypress — and doing
// precisely nothing, every single time, regardless of how correctly
// the viewport's Width/Height were sized. This is called unconditionally
// near the top of Update() so the persisted viewport's content — and
// therefore its scrollability — is never stale for whatever key
// handling runs next in the same Update() call.
func (m *Model) refreshViewportContent() {
	if !m.viewportReady {
		return
	}
	m.yamlViewport.SetContent(m.previewBody())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.refreshViewportContent()

	switch msg := msg.(type) {
	case saveResult:
		m.lastSave = msg
		if msg.err == nil {
			m.dirty = false
			m.currentMode = modeSaved
			m.syncViewportHeight()
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
		m.syncViewportHeight()
		return m, nil

	case clearSaveBannerMsg:
		if m.currentMode == modeSaved {
			m.currentMode = modeNormal
			m.syncViewportHeight()
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
		if !m.viewportReady {
			_, _, rightW := paneWidths(m.width)
			m.yamlViewport = viewport.New(rightW, 1) // sized properly below
			m.viewportReady = true
		}
		m.syncViewportHeight()

		// Keep the editable form's input widths in sync with the current
		// terminal size. Previously these were computed once in
		// initFormInputs() and never revisited, so resizing the terminal
		// while editing left the fields sized for the old width — a
		// direct cause of the border/content misalignment on resize.
		if m.currentMode == modeEditField && len(m.formInputs) > 0 {
			_, centerW, _ := paneWidths(m.width)
			fieldWidth := centerW - 14 - panePaddingOverhead // 12 for label + 2 for spacing, minus pane padding overhead
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
// Esc discards any changes made this session — confirming first if
// formHasChanges() says there actually are any — and returns to normal
// mode without them. Ctrl+S saves the form to the service and returns
// to normal mode. Neither one writes to disk; that's a separate,
// deliberate action via the main screen's Ctrl+S (see updateSaveAs).
func (m Model) updateEditField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// A discard confirmation is in progress — only y/n/enter/esc are
	// meaningful here; everything else is ignored rather than falling
	// through to the form fields underneath.
	if m.confirmingDiscardEdit {
		switch msg.String() {
		case "y", "Y", "enter":
			m.discardFormChanges()
			m.confirmingDiscardEdit = false
			m.currentMode = modeNormal
			return m, nil
		case "n", "N", "esc":
			// Back to editing, not back to the main screen — the
			// most likely next step is continuing to work, not
			// abandoning the edit entirely.
			m.confirmingDiscardEdit = false
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		// Esc discards rather than saves — but only asks for
		// confirmation if there's actually something to lose. Every
		// keystroke while editing is already applied to the live
		// config (see saveFormToService), so "discard" here means
		// restoring editSnapshot, not just skipping a save.
		if !m.formHasChanges() {
			m.currentMode = modeNormal
			return m, nil
		}
		m.confirmingDiscardEdit = true
		return m, nil

	case "ctrl+s":
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

	// contentHeight is the exact number of rows available to the 3
	// panes. Computed via availableContentHeight() rather than inline
	// here, specifically so the WindowSizeMsg handler (which sizes the
	// YAML preview's viewport) can call the exact same function instead
	// of maintaining its own separate approximation. Those two used to
	// drift out of sync — the viewport clamped scrolling against one
	// height while the pane rendered using a different one, which is
	// what made the preview's scrolling appear to do nothing and its
	// bottom border vanish. See availableContentHeight's doc comment.
	contentHeight := m.availableContentHeight()

	leftW, centerW, rightW := paneWidths(m.width)

	var body string
	if len(m.config.Services) == 0 {
		// A brand-new/empty config used to just render the normal
		// 3-pane skeleton with a one-line hint buried in each pane and
		// the rest of the screen dead blank space — not really a
		// "first thing you see" experience. This replaces that with an
		// actual welcome screen; it goes away the moment a service
		// exists (added, imported, or from a preset/stack), so it's
		// only ever seen once per session.
		body = m.renderWelcomeScreen(m.width, contentHeight)
	} else {
		left := m.renderLeftPane(leftW, contentHeight)
		center := m.renderCenterPane(centerW, contentHeight)
		right := m.renderRightPane(rightW, contentHeight)
		body = joinHorizontal(left, center, right)
	}

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

// availableContentHeight returns exactly how many content rows the 3
// main panes have to render into: total terminal rows minus the outer
// header, the help bar, and the save/error banner row (if one is
// showing).
//
// This is the single source of truth for that number. It used to be
// computed twice, independently: once inline in View() (to size the
// panes) and once via a separate, hand-tuned approximation in the
// WindowSizeMsg handler (to size the YAML preview's viewport). Those
// two formulas drifted out of sync the moment either side of the app
// changed shape — e.g. the outer header going from a 1-line bar to a
// 3-line bordered box — since the viewport's formula had no way to
// know about that change. The visible result was the YAML preview
// clamping its scroll offset against a stale height while the pane
// itself rendered using a different, freshly-computed one: scrolling
// looked like it did nothing, and the bottom border could vanish
// entirely when the two heights disagreed enough. Routing both call
// sites through this one function removes the possibility of that
// drift outright.
// paneBorderHeight is how many rows a pane's own border consumes (1
// top + 1 bottom = 2). availableContentHeight() returns the CONTENT
// budget the 3 panes are given — the height param renderLeftPane etc.
// treat as pre-border — so it has to reserve these 2 rows on top of
// the header/help bar/banner, or the pane row (content+border) ends up
// exactly 2 rows taller than the terminal has room for. That overflow
// doesn't get clipped from the bottom the way you'd expect in a
// scrolling terminal — bubbletea writes the whole frame via absolute
// cursor positioning, so when it's taller than the terminal the
// OS-level terminal buffer itself scrolls, pushing the top of the
// frame (the header) up and out of view instead. This was most visible
// whenever the help bar happened to wrap onto a second line, since
// that alone was often enough extra height to tip a borderline-tight
// terminal over into losing the header entirely.
const paneBorderHeight = 2

func (m Model) availableContentHeight() int {
	header := m.renderHeader()
	helpBar := m.renderHelpBar()

	bannerLines := 0
	if m.currentMode == modeSaved || m.lastSave.err != nil {
		bannerLines = 1
	}

	contentHeight := m.height - lipglossHeight(header) - lipglossHeight(helpBar) - bannerLines - paneBorderHeight
	if contentHeight < 1 {
		contentHeight = 1
	}
	return contentHeight
}

// syncViewportHeight keeps the YAML preview viewport's Width/Height
// matched to whatever availableContentHeight() currently computes.
// It's called from every place that can change that number — not just
// WindowSizeMsg (terminal resize), but also wherever the save/error
// banner appears or disappears, since that's a 1-row change too and
// nothing about a save happening fires a resize event. Missing any of
// these call sites would reintroduce exactly the stale-height mismatch
// availableContentHeight's doc comment describes, just triggered by a
// banner instead of a resize.
func (m *Model) syncViewportHeight() {
	if !m.viewportReady {
		return
	}
	_, _, rightW := paneWidths(m.width)
	viewportContentHeight := m.availableContentHeight() - 2 // paneHeader = title + divider
	if viewportContentHeight < 1 {
		viewportContentHeight = 1
	}
	// viewport.View() pads EVERY line to exactly m.Width internally
	// (it's built on its own lipgloss.NewStyle().Width(contentWidth)
	// call). Setting that to the pane's full `rightW` meant every
	// single line of YAML content ended up exactly `rightW` characters
	// wide — which is exactly the width the outer pane style then
	// word-wraps at (minus its own padding). The preview wasn't just
	// occasionally wrapping a too-long line; it was doubling nearly
	// every line in the file. See panePaddingOverhead's doc comment.
	viewportWidth := rightW - panePaddingOverhead
	if viewportWidth < 1 {
		viewportWidth = 1
	}
	m.yamlViewport.Width = viewportWidth
	m.yamlViewport.Height = viewportContentHeight
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
		if m.confirmingDiscardEdit {
			help = "y: discard • n/esc: keep editing"
		} else {
			help = "↑↓: move / switch field • enter: newline in list fields • tab: next field • ctrl+s: save • esc: discard"
		}
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

// panePaddingOverhead is how many columns basePaneStyle's own
// Padding(0, 1) consumes on each side (1 left + 1 right = 2 total).
//
// This matters more than it sounds like it should: lipgloss wraps
// content to (width - leftPadding - rightPadding), NOT to width
// itself — see style.go's Render, the "wrapAt := width - leftPadding -
// rightPadding" line. Any line built to fill a pane's content area to
// EXACTLY `width` characters — not shorter — silently gets word-wrapped
// into two lines the moment it passes through paneStyle().Width(width),
// because the real wrap boundary is width-2, not width.
//
// This was the actual root cause behind three things that looked like
// unrelated bugs: a stray 2-character line appearing under every
// pane's title divider (the divider was built at exactly `width`
// dashes), the YAML preview's lines effectively doubling up (the
// viewport pads every line to exactly its own Width internally, which
// was also set to the full `width`), and — as a direct consequence of
// that overflow — the preview pane's bottom border getting clipped off
// along with the excess. Every place that builds a line meant to span
// a pane's full content width needs to target width-panePaddingOverhead,
// not width.
const panePaddingOverhead = 2

// paneHeader renders a pane's title followed by a full-width divider
// line, visually separating the title from the body like a table
// header row. All three panes go through this so the title is always
// the first line of pane content and never gets mistaken for part of
// the scrollable body.
// paneHeader renders a pane's title followed by a full-width divider
// line, visually separating the title from the body like a table
// header row. All three panes go through this so the title is always
// the first line of pane content and never gets mistaken for part of
// the scrollable body.
//
// It's always exactly 2 lines — every caller that sizes something
// relative to a pane's body (most importantly the YAML viewport's own
// height, in syncViewportHeight) assumes that and subtracts a fixed 2.
// The title is truncated, not left to wrap, specifically to guarantee
// that: at narrow widths "Preview: docker-compose.yml" is long enough
// to wrap onto a second line on its own, which silently made paneHeader
// 3 lines tall instead of 2 — the viewport was then sized 1 row too
// tall for the space actually left under it, which is what clipped its
// bottom border off at narrow terminal widths.
// renderWelcomeScreen renders the first-launch screen shown in place of
// the 3-pane layout when the config has no services yet. It's centered
// in the same content area the panes would otherwise occupy, and
// disappears the moment a service exists by any means — added by hand,
// imported, or from a preset/stack.
func (m Model) renderWelcomeScreen(width, height int) string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorAccent).
		Render("komposer")

	tagline := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("Build a docker-compose.yml in seconds")

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(colorTitle)
	descStyle := lipgloss.NewStyle().Foreground(colorSubtle)

	row := func(key, desc string) string {
		return keyStyle.Render(fmt.Sprintf("%-8s", key)) + descStyle.Render(desc)
	}

	actionsBlock := lipgloss.JoinVertical(lipgloss.Left,
		row("a", "add a single service"),
		row("Ctrl+P", "browse presets and ready-made stacks"),
		row("Ctrl+O", "import an existing docker-compose.yml"),
	)

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("q to quit anytime")

	box := lipgloss.JoinVertical(lipgloss.Center,
		title,
		tagline,
		"",
		actionsBlock,
		"",
		hint,
	)

	// The box stays at its natural (compact) size on any terminal wide
	// enough for it — MaxWidth alone would have clipped raw bytes off
	// the right edge on a narrow terminal instead of wrapping (the same
	// lesson learned everywhere else in this file: it truncates, it
	// doesn't reflow). Width() is used instead so lipgloss's own wrap
	// engine can properly re-flow the text and keep the border intact
	// when the box has to shrink.
	//
	// This style has Padding(1, 4) — lipgloss wraps to (Width -
	// leftPadding - rightPadding), so the value passed to Width() has
	// to be the desired CONTENT budget plus that 8 back, or the content
	// wraps 8 columns earlier than intended even when there's plenty of
	// room. This is the exact same padding-eats-columns behavior
	// documented on panePaddingOverhead, just easy to re-trip over
	// because this box's padding (8) differs from the panes' (2).
	const boxPadding = 8 // Padding(1, 4): 4 cols each side
	const boxBorder = 2  // 1 col each side
	naturalWidth := lipgloss.Width(box)
	maxContentBudget := width - boxBorder - boxPadding
	if maxContentBudget < 20 {
		maxContentBudget = 20
	}
	contentBudget := naturalWidth
	if contentBudget > maxContentBudget {
		contentBudget = maxContentBudget
	}

	boxed := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 4).
		Width(contentBudget + boxPadding).
		Render(box)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, boxed)
}

func paneHeader(title string, width int) string {
	// paneTitleStyle has its own Padding(0, 1) — 2 more visible
	// characters added AFTER truncation. Budgeting only
	// width-panePaddingOverhead for the text itself would leave the
	// rendered (padded) title line exactly back at the outer wrap
	// boundary once that padding is added, right back to the same bug.
	titleWidth := width - panePaddingOverhead - 2
	divider := lipgloss.NewStyle().
		Foreground(colorMuted).
		Render(strings.Repeat("─", width-panePaddingOverhead))
	return paneTitleStyle.Render(truncateText(title, titleWidth)) + "\n" + divider
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
			// truncateText's budget subtracts panePaddingOverhead on
			// top of the 2 columns reserved for the cursor prefix — a
			// maximally long name would otherwise produce a line of
			// exactly `width` visible characters, which hits the same
			// wrap-at-width-minus-padding issue paneHeader's divider
			// did (see panePaddingOverhead's doc comment).
			body.WriteString(cursor + truncateText(names[i], width-2-panePaddingOverhead) + "\n")
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
	body := m.previewBody()

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
	// Width here must be width-panePaddingOverhead, not width: this
	// inner style has no padding of its own, so it happily wraps/pads
	// every line to exactly `width` characters — but that then gets
	// embedded in the outer pane's content, which DOES have padding,
	// so its own wrap boundary is width-panePaddingOverhead. Lines
	// built to exactly `width` were getting caught by that outer wrap
	// and split in two. See panePaddingOverhead's doc comment.
	wrapStyle := lipgloss.NewStyle().Width(width - panePaddingOverhead)
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
