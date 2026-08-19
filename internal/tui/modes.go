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
}
