package tui

import (
	"strings"

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

// initFormInputs creates a fresh set of textinput models for editing the
// selected service's configuration.
func (m *Model) initFormInputs() {
	m.formInputs = make([]textinput.Model, numFormFields)

	placeholders := []string{
		"postgres:15",
		"./path/to/Dockerfile or . for context",
		"8080:80, 443:443",
		"KEY=VALUE, DEBUG=true",
		"./data:/var/lib/data",
		"unless-stopped",
	}

	// Calculate available width for form fields (center pane width - label width - padding)
	_, centerW, _ := paneWidths(m.width)
	fieldWidth := centerW - 14 // 12 for label + 2 for spacing
	if fieldWidth < 20 {
		fieldWidth = 20
	}

	for i := range m.formInputs {
		ti := textinput.New()
		ti.Placeholder = placeholders[i]
		ti.Width = fieldWidth
		m.formInputs[i] = ti
	}

	// Load current service values into inputs if a service is selected
	if len(m.config.Services) > 0 && m.selected < len(m.config.Services) {
		entry := m.config.Services[m.selected]
		c := entry.Config

		m.formInputs[fieldImage].SetValue(c.Image)
		m.formInputs[fieldBuild].SetValue(c.Build)
		m.formInputs[fieldPorts].SetValue(strings.Join(c.Ports, ", "))
		m.formInputs[fieldEnvironment].SetValue(strings.Join(c.Environment, ", "))
		m.formInputs[fieldVolumes].SetValue(strings.Join(c.Volumes, ", "))
		m.formInputs[fieldRestart].SetValue(c.Restart)
	}

	// Focus first field when entering edit mode
	if len(m.formInputs) > 0 {
		m.formInputs[0].Focus()
		m.focusedFormField = 0
	}
}

// saveFormToService applies the values from formInputs back to the currently
// selected service's configuration.
func (m *Model) saveFormToService() {
	if len(m.config.Services) == 0 || m.selected >= len(m.config.Services) {
		return
	}

	c := m.config.Services[m.selected].Config

	c.Image = strings.TrimSpace(m.formInputs[fieldImage].Value())
	c.Build = strings.TrimSpace(m.formInputs[fieldBuild].Value())

	c.Ports = splitTrim(m.formInputs[fieldPorts].Value(), ",")
	c.Environment = splitTrim(m.formInputs[fieldEnvironment].Value(), ",")
	c.Volumes = splitTrim(m.formInputs[fieldVolumes].Value(), ",")

	c.Restart = strings.TrimSpace(m.formInputs[fieldRestart].Value())
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

// nextFormField moves focus to the next field in the form, wrapping around.
func (m *Model) nextFormField() {
	if len(m.formInputs) == 0 {
		return
	}
	m.formInputs[m.focusedFormField].Blur()
	m.focusedFormField = (m.focusedFormField + 1) % len(m.formInputs)
	m.formInputs[m.focusedFormField].Focus()
}

// prevFormField moves focus to the previous field in the form, wrapping around.
func (m *Model) prevFormField() {
	if len(m.formInputs) == 0 {
		return
	}
	m.formInputs[m.focusedFormField].Blur()
	m.focusedFormField = (m.focusedFormField - 1 + len(m.formInputs)) % len(m.formInputs)
	m.formInputs[m.focusedFormField].Focus()
}
