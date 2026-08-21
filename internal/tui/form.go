package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
)

// formField identifies which field in the service config form is being edited.
type formField int

const (
	fieldImage formField = iota
	fieldBuild
	fieldPorts
	fieldEnvironment
	fieldVolumes
	fieldRestart
	numFormFields
)

// isListField reports whether a field holds a list of values (edited
// one item per line, via textarea) rather than a single value (edited
// on one line, via textinput).
func isListField(f formField) bool {
	switch f {
	case fieldPorts, fieldEnvironment, fieldVolumes:
		return true
	default:
		return false
	}
}

// syncListFieldHeight sizes a list field's textarea to exactly as many
// rows as it currently has lines, instead of a fixed height. It used to
// be a fixed 3 rows with internal scrolling — past 3 lines, the ones
// you'd already typed scrolled up out of view while you were still
// adding more. Growing the field itself means every line stays visible;
// the pane's own MaxHeight (set in renderCenterPane) is still there as
// a backstop if the whole form ends up taller than the terminal.
func syncListFieldHeight(ta *textarea.Model) {
	h := ta.LineCount()
	if h < 1 {
		h = 1
	}
	ta.SetHeight(h)
}

// initFormInputs creates a fresh set of input models for editing the
// selected service's configuration. Image/Build/Restart get a
// single-line textinput; Ports/Environment/Volumes get a multi-line
// textarea, one list item per line, so you can add and remove entries
// with Enter instead of editing one long comma-separated string.
func (m *Model) initFormInputs() {
	m.formInputs = make([]textinput.Model, numFormFields)
	m.formAreas = make([]textarea.Model, numFormFields)

	singleLinePlaceholders := map[formField]string{
		fieldImage:   "postgres:15",
		fieldBuild:   "./path/to/Dockerfile or . for context",
		fieldRestart: "unless-stopped",
	}
	listPlaceholders := map[formField]string{
		fieldPorts:       "8080:80",
		fieldEnvironment: "KEY=VALUE",
		fieldVolumes:     "./data:/var/lib/data",
	}

	// Calculate available width for form fields (center pane width - label width - padding)
	_, centerW, _ := paneWidths(m.width)
	fieldWidth := centerW - 14 // 12 for label + 2 for spacing
	if fieldWidth < 20 {
		fieldWidth = 20
	}

	for i := formField(0); i < numFormFields; i++ {
		if isListField(i) {
			ta := textarea.New()
			ta.Placeholder = listPlaceholders[i] + " (one per line)"
			ta.ShowLineNumbers = false
			ta.Prompt = "" // no left-margin glyph — keep width math exact
			ta.CharLimit = 0
			ta.SetWidth(fieldWidth)
			m.formAreas[i] = ta
		} else {
			ti := textinput.New()
			ti.Placeholder = singleLinePlaceholders[i]
			ti.Width = fieldWidth
			m.formInputs[i] = ti
		}
	}

	// Load current service values into inputs if a service is selected
	if len(m.config.Services) > 0 && m.selected < len(m.config.Services) {
		entry := m.config.Services[m.selected]
		c := entry.Config

		m.formInputs[fieldImage].SetValue(c.Image)
		m.formInputs[fieldBuild].SetValue(c.Build)
		m.formInputs[fieldRestart].SetValue(c.Restart)

		m.formAreas[fieldPorts].SetValue(strings.Join(c.Ports, "\n"))
		m.formAreas[fieldEnvironment].SetValue(strings.Join(c.Environment, "\n"))
		m.formAreas[fieldVolumes].SetValue(strings.Join(c.Volumes, "\n"))
	}

	for i := range m.formAreas {
		if isListField(formField(i)) {
			syncListFieldHeight(&m.formAreas[i])
		}
	}

	// Focus first field when entering edit mode
	m.focusedFormField = 0
	m.formInputs[fieldImage].Focus()
}

// saveFormToService applies the values from the form fields back to the
// currently selected service's configuration.
func (m *Model) saveFormToService() {
	if len(m.config.Services) == 0 || m.selected >= len(m.config.Services) {
		return
	}

	c := m.config.Services[m.selected].Config

	c.Image = strings.TrimSpace(m.formInputs[fieldImage].Value())
	c.Build = strings.TrimSpace(m.formInputs[fieldBuild].Value())
	c.Restart = strings.TrimSpace(m.formInputs[fieldRestart].Value())

	c.Ports = splitLines(m.formAreas[fieldPorts].Value())
	c.Environment = splitLines(m.formAreas[fieldEnvironment].Value())
	c.Volumes = splitLines(m.formAreas[fieldVolumes].Value())

	// This is the single choke point every edit-mode keystroke runs
	// through (see updateEditField), so it's also the simplest correct
	// place to mark the config dirty — covers every field, without
	// needing a separate flag at each call site.
	m.dirty = true
}

// splitTrim splits a comma-separated string and trims whitespace from each part,
// filtering out empty strings.
func splitTrim(s, sep string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitLines splits a newline-separated string (as produced by a
// textarea list field) into trimmed, non-empty items — the equivalent
// of splitTrim for fields that are now edited one item per line
// instead of comma-separated on one line.
func splitLines(s string) []string {
	return splitTrim(s, "\n")
}

// nextFormField moves focus to the next field in the form, wrapping
// around. The new field's cursor lands at its start, matching how
// tabbing forward through a normal form works.
// nextFormField moves focus to the next field in the form, wrapping
// around. The new field's cursor lands at its end — same as
// prevFormField below — since always landing at the end (rather than
// the start when moving forward and the end when moving back) is what
// actually reads as consistent while arrowing through the form.
func (m *Model) nextFormField() {
	blurField(m, formField(m.focusedFormField))
	m.focusedFormField = int((formField(m.focusedFormField) + 1) % numFormFields)
	f := formField(m.focusedFormField)
	focusField(m, f)
	cursorEnd(m, f)
}

// prevFormField moves focus to the previous field in the form, wrapping
// around. The cursor lands at its end, same as nextFormField.
func (m *Model) prevFormField() {
	blurField(m, formField(m.focusedFormField))
	m.focusedFormField = int((formField(m.focusedFormField) - 1 + numFormFields) % numFormFields)
	f := formField(m.focusedFormField)
	focusField(m, f)
	cursorEnd(m, f)
}

func blurField(m *Model, f formField) {
	if isListField(f) {
		m.formAreas[f].Blur()
	} else {
		m.formInputs[f].Blur()
	}
}

func focusField(m *Model, f formField) {
	if isListField(f) {
		m.formAreas[f].Focus()
	} else {
		m.formInputs[f].Focus()
	}
}

func cursorEnd(m *Model, f formField) {
	if isListField(f) {
		m.formAreas[f].CursorEnd()
	} else {
		m.formInputs[f].CursorEnd()
	}
}
