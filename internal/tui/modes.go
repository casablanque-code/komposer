package tui

import (
	"github.com/casablanque-code/komposer/pkg/composer"
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
	modeSecretStrategy
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
	// secretRefs is parallel to warnings: secretRefs[i] is non-nil
	// when warnings[i] is one classifySecretEnv flagged (empty or
	// hardcoded) and therefore something ConvertSecretToEnvFile /
	// ConvertSecretToComposeSecret can act on (rendered with an arrow marker instead of a circle - see buildValidationBodyLines).
	// Warnings with no corresponding fix (e.g. the Postgres-specific
	// "no POSTGRES_PASSWORD set at all" advisory, which has no
	// existing environment entry to convert) are left nil here.
	secretRefs []*composer.HardcodedSecret
	// selectedWarning is which entry in warnings/secretRefs is
	// currently under the cursor — Up/Down move it directly, one
	// warning at a time, auto-scrolling it into view (see
	// updateValidation/ensureValidationLineVisible). -1 when there are
	// no warnings at all. A warning with no fix behind it
	// (secretRefs[selectedWarning] == nil) can still be the current
	// selection — landing there just makes Enter a no-op instead of
	// opening the picker.
	selectedWarning int
	// actionMessage shows the result of the most recent secret
	// conversion (success or failure) as a banner at the top of the
	// report. Cleared whenever showValidation next runs.
	actionMessage    string
	actionMessageErr bool
	// specValid and specIssues hold the result of checking the
	// rendered document against the official Compose Specification
	// JSON Schema — see ComposeConfig.ValidateAgainstSpec. This is a
	// separate question from errors/warnings above, which are
	// komposer's own opinionated checks, not the Compose Spec itself.
	specValid  bool
	specIssues []string
	scroll     int
}

// secretStrategyDialog holds state for the "how should this secret be
// stored" picker, opened from the validation dialog (Enter, on a
// selected warning with a non-nil secretRefs entry). service/key/empty
// identify
// which environment entry it's acting on; selected is the currently
// highlighted option (0 = .env, 1 = Compose secret, 2 = keep as is).
type secretStrategyDialog struct {
	service  string
	key      string
	empty    bool
	selected int
}

func newSecretStrategyDialog(service, key string, empty bool) secretStrategyDialog {
	return secretStrategyDialog{service: service, key: key, empty: empty, selected: 0}
}

// secretActionResult carries the outcome of applying a secret
// conversion (see Model.convertSecretCmd) back into Update() as a
// tea.Msg, the same way saveResult does for Ctrl+S.
type secretActionResult struct {
	message string
	err     error
}

type importDialog struct {
	pathInput textinput.Model
}

func newImportDialog() importDialog {
	ti := textinput.New()
	ti.Placeholder = "compose.yaml"
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
	ti.Placeholder = "compose.yaml"
	ti.SetValue("compose.yaml")
	ti.Focus()
	ti.CursorEnd()
	ti.CharLimit = 256
	ti.Width = 50
	return saveAsDialog{pathInput: ti, quitAfterSave: quitAfterSave}
}
