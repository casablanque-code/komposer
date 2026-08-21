package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
)

// mode represents the current UI state/mode.
type mode int

const (
	modeNormal mode = iota
	modeAddService
	modeConfirmDelete
	modeEditField
	modePresetPicker
	modeSaving
	modeSaved
	modeValidation
	modeImport
	modeSaveAs
)

// addServiceDialog holds state for the "add service" modal dialog.
type addServiceDialog struct {
	nameInput textinput.Model
}

func newAddServiceDialog() addServiceDialog {
	ti := textinput.New()
	ti.Placeholder = "service-name"
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = 30
	return addServiceDialog{nameInput: ti}
}

// confirmDeleteDialog holds state for the delete confirmation prompt.
type confirmDeleteDialog struct {
	serviceName string
}

type presetPickerDialog struct {
	selected     int
	nameInput    textinput.Model
	stage        int // 0 = pick preset, 1 = enter name
	chosenPreset int
	// tab selects which list stage 0 shows: 0 = single-service Presets,
	// 1 = multi-service Stacks. Stacks skip the name-input stage
	// entirely (see updatePresetPicker) since asking for one name
	// wouldn't make sense for something that adds several services at
	// once — the whole point is picking it and being done.
	tab int
}

func newPresetPickerDialog() presetPickerDialog {
	ti := textinput.New()
	ti.Placeholder = "service-name"
	ti.CharLimit = 64
	ti.Width = 30
	return presetPickerDialog{
		selected:  0,
		nameInput: ti,
		stage:     0,
	}
}

type saveResult struct {
	filename string
	err      error
	// quitAfterSave carries through from the dialog that triggered this
	// save: when true (the dialog was opened because the user pressed
	// 'q' with unsaved changes), a successful write should exit the
	// program afterward instead of returning to the normal view.
	quitAfterSave bool
}

type validationDialog struct {
	errors   []string
	warnings []string
	scroll   int
}

type importDialog struct {
	pathInput textinput.Model
}

func newImportDialog() importDialog {
	ti := textinput.New()
	ti.Placeholder = "docker-compose.yml"
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50
	return importDialog{pathInput: ti}
}

// saveAsDialog holds state for the explicit "save to disk" prompt. It's
// shown whenever a save actually happens — from Ctrl+S on the main
// screen, and from 'q'/Ctrl+C when there are unsaved changes — instead
// of silently writing to a hardcoded "docker-compose.yml" and flashing
// a banner, which wasn't clear about what had just happened or where
// the file went.
type saveAsDialog struct {
	pathInput     textinput.Model
	quitAfterSave bool
	// confirmingOverwrite and pendingPath hold the state for the
	// "file already exists, overwrite?" step — see updateSaveAs.
	confirmingOverwrite bool
	pendingPath         string
}

func newSaveAsDialog(quitAfterSave bool) saveAsDialog {
	ti := textinput.New()
	ti.Placeholder = "docker-compose.yml"
	ti.SetValue("docker-compose.yml")
	ti.Focus()
	ti.CursorEnd()
	ti.CharLimit = 256
	ti.Width = 50
	return saveAsDialog{pathInput: ti, quitAfterSave: quitAfterSave}
}
