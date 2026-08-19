package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/casablanque-code/komposer/pkg/composer"
)

// dialogContentWidth returns a safe content width for modal dialogs: wide
// enough to be readable, but never wider than the terminal can actually
// show. Every dialog's wrappable text should be rendered at this width
// before being boxed, so long strings (preset descriptions, validation
// messages) wrap instead of forcing the dialog wider than the screen —
// lipgloss.Place() does not shrink oversized content, it just breaks.
func dialogContentWidth(termWidth int) int {
	const preferred = 56  // comfortable reading width
	const margin = 6      // border(2) + padding(4) the dialog box adds

	max := termWidth - margin
	if max < 20 {
		max = 20 // absolute floor so tiny terminals don't collapse to 0
	}
	if preferred < max {
		return preferred
	}
	return max
}

// renderDialogBox wraps already-built content in the standard bordered
// dialog chrome and centers it on screen, clamping its total width so it
// can never exceed the terminal (the previous overflow was the root cause
// of the preset picker "falling apart" on narrower terminals).
func renderDialogBox(termWidth, termHeight int, borderColor lipgloss.Color, content string) string {
	dialog := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		MaxWidth(termWidth).
		Render(content)

	return lipgloss.Place(termWidth, termHeight, lipgloss.Center, lipgloss.Center, dialog)
}

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

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("Enter: confirm • Esc: cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		prompt,
		input,
		"",
		hint,
	)

	return renderDialogBox(m.width, m.height, colorAccent, content)
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
		Render("Y: confirm • N/Esc: cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		prompt,
		"",
		hint,
	)

	return renderDialogBox(m.width, m.height, colorDanger, content)
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

	contentWidth := dialogContentWidth(m.width)

	var rows []string
	for i, preset := range composer.Presets {
		selected := i == m.presetPicker.selected

		cursor := "  "
		nameStyle := lipgloss.NewStyle().Bold(true)
		descColor := colorSubtle
		if selected {
			cursor = "▸ "
			nameStyle = nameStyle.Foreground(colorAccent)
			descColor = colorTitle
		}

		// Every row is exactly two lines: the name, then one
		// single-line, truncated description. Previously the
		// description was word-wrapped, which made long presets
		// (PostgreSQL, MySQL) three lines tall while short ones
		// (Redis, MongoDB) stayed at two — that mismatch made the
		// cursor look like it was drifting onto description lines
		// as you scrolled. Fixed row height removes the ambiguity.
		nameLine := cursor + nameStyle.Render(preset.Name)
		descLine := "  " + lipgloss.NewStyle().
			Foreground(descColor).
			Render(truncateText(preset.Description, contentWidth-2))

		rows = append(rows, nameLine+"\n"+descLine)
	}

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("↑↓: navigate • enter: select • esc: cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		strings.Join(rows, "\n\n"),
		"",
		hint,
	)

	return renderDialogBox(m.width, m.height, colorAccent, content)
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

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("Enter: confirm • Esc: back")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		presetInfo,
		"",
		prompt,
		input,
		"",
		hint,
	)

	return renderDialogBox(m.width, m.height, colorAccent, content)
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

	contentWidth := dialogContentWidth(m.width)
	errStyle := lipgloss.NewStyle().Width(contentWidth)

	var errorLines []string
	if len(m.validationDialog.errors) == 0 {
		errorLines = append(errorLines, lipgloss.NewStyle().
			Foreground(colorSuccess).
			Render("✓ All checks passed!"))
	} else {
		for _, err := range m.validationDialog.errors {
			// Wrap each error individually: these come from
			// ValidationError.Error() and can easily run past 60
			// columns (service name + field + message), which used to
			// force the dialog wider than the terminal.
			errorLines = append(errorLines, errStyle.Render("• "+err))
		}
	}

	errorText := strings.Join(errorLines, "\n")

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Render("Esc: close")

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

	return renderDialogBox(m.width, m.height, borderColor, content)
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
		Render("Enter: import • Esc: cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		prompt,
		input,
		"",
		hint,
	)

	return renderDialogBox(m.width, m.height, colorAccent, content)
}
