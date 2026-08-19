package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/casablanque-code/komposer/pkg/composer"
)

// renderAddServiceDialog renders the modal dialog for adding a new service.
func (m Model) renderAddServiceDialog() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Render("Add New Service")

	prompt := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("Enter service name:")

	input := m.addDialog.nameInput.View()

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		prompt,
		input,
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Width(40).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

// renderConfirmDeleteDialog renders the confirmation dialog for deleting a service.
func (m Model) renderConfirmDeleteDialog() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorDanger).
		Render("Confirm Delete")

	prompt := fmt.Sprintf("Delete service '%s'?", m.confirmDelete.serviceName)

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("Press Y to confirm, N to cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		prompt,
		"",
		hint,
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorDanger).
		Padding(1, 2).
		Width(40).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

func (m Model) renderPresetPickerDialog() string {
	if m.presetPicker.stage == 0 {
		return m.renderPresetList()
	}
	return m.renderPresetNameInput()
}

func (m Model) renderPresetList() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Render("Choose Preset")

	var items []string
	for i, preset := range composer.Presets {
		cursor := "  "
		if i == m.presetPicker.selected {
			cursor = "> "
		}
		line := cursor + preset.Name + " — " + preset.Description
		items = append(items, line)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		strings.Join(items, "\n"),
		"",
		lipgloss.NewStyle().Foreground(colorSubtle).Render("↑↓: navigate • enter: select • esc: cancel"),
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Width(70).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

func (m Model) renderPresetNameInput() string {
	preset := composer.Presets[m.presetPicker.chosenPreset]

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Render("Name Your Service")

	presetInfo := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render(fmt.Sprintf("Preset: %s", preset.Name))

	prompt := "Service name:"

	input := m.presetPicker.nameInput.View()

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		presetInfo,
		"",
		prompt,
		input,
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Width(50).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

// renderEditableForm renders the interactive form with text inputs.
func (m Model) renderEditableForm() string {
	if len(m.formInputs) == 0 {
		return ""
	}

	labels := []string{"Image:", "Build:", "Ports:", "Environment:", "Volumes:", "Restart:"}

	var lines []string
	for i, label := range labels {
		labelStyle := lipgloss.NewStyle().
			Foreground(colorSubtle).
			Width(12).
			Align(lipgloss.Right)

		line := labelStyle.Render(label) + " " + m.formInputs[i].View()
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderValidationDialog() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorDanger).
		Render("Validation Errors")

	var errorLines []string
	if len(m.validationDialog.errors) == 0 {
		errorLines = append(errorLines, lipgloss.NewStyle().
			Foreground(colorSuccess).
			Render("✓ All checks passed!"))
	} else {
		for _, err := range m.validationDialog.errors {
			errorLines = append(errorLines, "• "+err)
		}
	}

	errorText := strings.Join(errorLines, "\n")

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("Press ESC to close")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		errorText,
		"",
		hint,
	)

	borderColor := colorDanger
	if len(m.validationDialog.errors) == 0 {
		borderColor = colorSuccess
	}

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(60).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

func (m Model) renderImportDialog() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Render("Import docker-compose.yml")

	prompt := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("Enter path to docker-compose.yml:")

	input := m.importDialog.pathInput.View()

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("Press Enter to import, ESC to cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		prompt,
		input,
		"",
		hint,
	)

	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Width(60).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}
