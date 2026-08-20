package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/casablanque-code/komposer/pkg/composer"
)

// dialogContentWidth returns the exact width for dialog content.
// All text inside dialogs MUST be rendered at this width.
func dialogContentWidth(termWidth int) int {
	const preferred = 60
	const padding = 4  // 2 chars padding on each side
	const border = 2   // 1 char border on each side

	maxContent := termWidth - padding - border
	if maxContent < 20 {
		return 20
	}
	if preferred < maxContent {
		return preferred
	}
	return maxContent
}

// renderDialogBox wraps already-built content in the standard bordered
// dialog chrome and centers it on screen. MaxWidth is a hard backstop:
// every line of content should already be built at dialogContentWidth,
// but if any line slips through wider than that (e.g. a value that
// wasn't truncated), this keeps the box from being rendered wider than
// the terminal, which is what tears the border apart on narrow widths.
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
	w := dialogContentWidth(m.width)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Width(w).
		Render("Add New Service")

	prompt := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render("Enter service name:")

	input := m.addDialog.nameInput.View()

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
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
	w := dialogContentWidth(m.width)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorDanger).
		Width(w).
		Render("Confirm Delete")

	prompt := lipgloss.NewStyle().
		Width(w).
		Render(fmt.Sprintf("Delete service '%s'?", m.confirmDelete.serviceName))

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
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
	w := dialogContentWidth(m.width)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Width(w).
		Render("Choose Preset")

	var rows []string
	for i, preset := range composer.Presets {
		selected := i == m.presetPicker.selected

		// Simple, clean rendering without extra complexity
		cursor := "  "
		if selected {
			cursor = "▸ "
		}

		// Name line
		nameStyle := lipgloss.NewStyle()
		if selected {
			nameStyle = nameStyle.Foreground(colorAccent).Bold(true)
		} else {
			nameStyle = nameStyle.Bold(true)
		}

		// Truncate the name too, not just the description — this line
		// previously had no width cap at all, so it was the one piece of
		// content in the whole app that could grow past the dialog's
		// width with nothing to stop it.
		nameLine := cursor + nameStyle.Render(truncateText(preset.Name, w-4))

		// Description line
		descStyle := lipgloss.NewStyle()
		if selected {
			descStyle = descStyle.Foreground(colorTitle)
		} else {
			descStyle = descStyle.Foreground(colorSubtle)
		}

		descText := truncateText(preset.Description, w-4)
		descLine := "  " + descStyle.Render(descText)

		// Add to rows as single block
		rows = append(rows, nameLine+"\n"+descLine)
	}

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render("↑↓: navigate • enter: select • esc: cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		strings.Join(rows, "\n"),
		"",
		hint,
	)

	return renderDialogBox(m.width, m.height, colorAccent, content)
}

func (m Model) renderPresetNameInput() string {
	w := dialogContentWidth(m.width)
	preset := composer.Presets[m.presetPicker.chosenPreset]

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Width(w).
		Render("Name Your Service")

	presetInfo := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render(fmt.Sprintf("Preset: %s", preset.Name))

	prompt := lipgloss.NewStyle().
		Width(w).
		Render("Service name:")

	input := m.presetPicker.nameInput.View()

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
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
// List fields (Ports/Environment/Volumes) render as a multi-line
// textarea instead of a single-line input — JoinHorizontal(Top, ...)
// keeps the label aligned to the field's first line rather than
// vertically centered against its full height.
func (m Model) renderEditableForm() string {
	if len(m.formInputs) == 0 && len(m.formAreas) == 0 {
		return ""
	}

	labels := []string{"Image:", "Build:", "Ports:", "Environment:", "Volumes:", "Restart:"}

	var lines []string
	for i, label := range labels {
		f := formField(i)
		labelStyle := lipgloss.NewStyle().
			Foreground(colorSubtle).
			Width(12).
			Align(lipgloss.Right)

		var field string
		if isListField(f) {
			field = m.formAreas[i].View()
		} else {
			field = m.formInputs[i].View()
		}

		line := lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render(label)+" ", field)
		lines = append(lines, line)
	}

	lines = append(lines, "", helpStyle.Render(
		"Tab/Shift+Tab: switch field • Enter: newline in list fields • Ctrl+S: save • Esc: done"))

	return strings.Join(lines, "\n")
}

func (m Model) renderValidationDialog() string {
	w := dialogContentWidth(m.width)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorDanger).
		Width(w).
		Render("Validation Errors")

	var errorLines []string
	if len(m.validationDialog.errors) == 0 {
		errorLines = append(errorLines, lipgloss.NewStyle().
			Foreground(colorSuccess).
			Width(w).
			Render("✓ All checks passed!"))
	} else {
		for _, err := range m.validationDialog.errors {
			wrapped := lipgloss.NewStyle().
				Width(w).
				Render("• " + err)
			errorLines = append(errorLines, wrapped)
		}
	}

	errorText := strings.Join(errorLines, "\n")

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
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
	w := dialogContentWidth(m.width)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Width(w).
		Render("Import docker-compose.yml")

	prompt := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render("Enter path to docker-compose.yml:")

	input := m.importDialog.pathInput.View()

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
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
