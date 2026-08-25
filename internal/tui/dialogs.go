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
	const padding = 4 // 2 chars padding on each side
	const border = 2  // 1 char border on each side

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
	if m.presetPicker.stage == 1 {
		return m.renderPresetNameInput()
	}
	return m.renderPresetList()
}

// presetListItem is the shared row shape for both the Presets tab and
// the Stacks tab — same visual structure (name + description), just a
// different backing list.
type presetListItem struct {
	Name        string
	Description string
}

func (m Model) renderPresetList() string {
	w := dialogContentWidth(m.width)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Width(w).
		Render("Add to compose.yml")

	tabs := renderPickerTabs(m.presetPicker.tab, w)

	var items []presetListItem
	if m.presetPicker.tab == 1 {
		for _, s := range composer.Stacks {
			items = append(items, presetListItem{Name: s.Name, Description: s.Description})
		}
	} else {
		for _, p := range composer.Presets {
			items = append(items, presetListItem{Name: p.Name, Description: p.Description})
		}
	}
	rows := renderPickerRows(items, m.presetPicker.selected, w)

	hintText := "↑↓: navigate • ←→: switch tab • enter: select • esc: cancel"
	if m.presetPicker.tab == 1 {
		hintText = "↑↓: navigate • ←→: switch tab • enter: add stack • esc: cancel"
	}
	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render(hintText)

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		tabs,
		"",
		rows,
		"",
		hint,
	)

	return renderDialogBox(m.width, m.height, colorAccent, content)
}

// renderPickerTabs renders the "Presets / Stacks" tab bar shown above
// the list in stage 0 of the preset picker, with a divider underneath
// — the same title+divider convention used by the three main panes
// (see paneHeader), so this reads as part of the same visual language
// instead of a one-off.
func renderPickerTabs(active int, w int) string {
	labels := []string{"Presets", "Stacks"}
	var rendered []string
	for i, label := range labels {
		style := lipgloss.NewStyle().Padding(0, 1)
		if i == active {
			style = style.Bold(true).Foreground(colorAccent).Underline(true)
		} else {
			style = style.Foreground(colorSubtle)
		}
		rendered = append(rendered, style.Render(label))
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
	divider := lipgloss.NewStyle().
		Foreground(colorMuted).
		Render(strings.Repeat("─", w))
	return bar + "\n" + divider
}

// renderPickerRows renders a cursor + name + description block per
// item — the row shape both the Presets tab and the Stacks tab use.
// pickerVisibleRows is how many items the preset/stack picker shows at
// once. With up to ~17 stacks now in the catalog, showing everything
// unconditionally would make the dialog taller than most terminals —
// this windows the list around the selected item instead, with
// "N more above/below" markers so it's clear there's more to scroll to.
const pickerVisibleRows = 6

func renderPickerRows(items []presetListItem, selected int, w int) string {
	if len(items) == 0 {
		return helpStyle.Render("(none)")
	}

	visible := pickerVisibleRows
	if visible > len(items) {
		visible = len(items)
	}

	// Center the window on the selected item, clamped so it never
	// scrolls past either end of the list.
	offset := 0
	if len(items) > visible {
		offset = selected - visible/2
		if offset < 0 {
			offset = 0
		}
		if offset > len(items)-visible {
			offset = len(items) - visible
		}
	}

	var rows []string

	if offset > 0 {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(colorSubtle).
			Width(w).
			Render(fmt.Sprintf("  ^ %d more above", offset)))
	}

	for i := offset; i < offset+visible; i++ {
		item := items[i]
		sel := i == selected

		cursor := "  "
		if sel {
			cursor = "> "
		}

		nameStyle := lipgloss.NewStyle().Bold(true)
		if sel {
			nameStyle = nameStyle.Foreground(colorAccent)
		}
		nameLine := cursor + nameStyle.Render(truncateText(item.Name, w-4))

		descStyle := lipgloss.NewStyle().Foreground(colorSubtle)
		if sel {
			descStyle = descStyle.Foreground(colorTitle)
		}
		descLine := "  " + descStyle.Render(truncateText(item.Description, w-4))

		rows = append(rows, nameLine+"\n"+descLine)
	}

	if remaining := len(items) - (offset + visible); remaining > 0 {
		rows = append(rows, lipgloss.NewStyle().
			Foreground(colorSubtle).
			Width(w).
			Render(fmt.Sprintf("  v %d more below", remaining)))
	}

	return strings.Join(rows, "\n")
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

	if m.confirmingDiscardEdit {
		return lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Bold(true).Foreground(colorWarning).Render("Discard changes to this service?"),
			"",
			helpStyle.Render("y: discard • n/esc: keep editing"),
		)
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
		"Tab/Shift+Tab: switch field • Enter: newline in list fields • Ctrl+S: save • Esc: discard"))

	return strings.Join(lines, "\n")
}

func (m Model) renderValidationDialog() string {
	w := dialogContentWidth(m.width)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Width(w).
		Render("Validation")

	var sections []string

	if len(m.validationDialog.errors) == 0 && len(m.validationDialog.warnings) == 0 {
		sections = append(sections, lipgloss.NewStyle().
			Foreground(colorSuccess).
			Width(w).
			Render("[OK] All checks passed!"))
	}

	if len(m.validationDialog.errors) > 0 {
		errHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(colorDanger).
			Width(w).
			Render(fmt.Sprintf("Errors (%d) - must fix before this is valid compose:", len(m.validationDialog.errors)))
		sections = append(sections, errHeader)
		for _, err := range m.validationDialog.errors {
			sections = append(sections, lipgloss.NewStyle().
				Foreground(colorDanger).
				Width(w).
				Render("• "+err))
		}
	}

	if len(m.validationDialog.warnings) > 0 {
		if len(sections) > 0 {
			sections = append(sections, "")
		}
		warnHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWarning).
			Width(w).
			Render(fmt.Sprintf("Warnings (%d) - valid, but worth a look:", len(m.validationDialog.warnings)))
		sections = append(sections, warnHeader)
		for _, warning := range m.validationDialog.warnings {
			sections = append(sections, lipgloss.NewStyle().
				Foreground(colorWarning).
				Width(w).
				Render("• "+warning))
		}
	}

	body := strings.Join(sections, "\n")

	// Window the body to whatever room is actually available, the same
	// windowing approach used by the preset/stack picker and the
	// services list — otherwise a config with enough services and
	// warnings to fill more than one screen just grew the dialog past
	// the terminal's height with nothing to scroll it, pushing content
	// (and the box's own bottom border) off-screen. Splitting the
	// ALREADY-rendered body by "\n" (rather than windowing by logical
	// section) matters because a single warning can wrap onto more than
	// one physical line at dialogContentWidth — windowing pre-wrap would
	// undercount how many screen rows a long message actually takes.
	bodyLines := strings.Split(body, "\n")

	// Fixed chrome around the body: border(2) + Padding(1,2)(2) +
	// title+blank(2) + blank+hint(2) = 8 rows not available to the body.
	const dialogChrome = 8
	visible := m.height - dialogChrome
	if visible < 3 {
		visible = 3
	}

	scroll := m.validationDialog.scroll
	if scroll > len(bodyLines)-visible {
		scroll = len(bodyLines) - visible
	}
	if scroll < 0 {
		scroll = 0
	}

	var windowed []string
	if scroll > 0 {
		windowed = append(windowed, lipgloss.NewStyle().
			Foreground(colorSubtle).
			Width(w).
			Render(fmt.Sprintf("^ %d more above", scroll)))
	}
	end := scroll + visible
	if end > len(bodyLines) {
		end = len(bodyLines)
	}
	windowed = append(windowed, bodyLines[scroll:end]...)
	if remaining := len(bodyLines) - end; remaining > 0 {
		windowed = append(windowed, lipgloss.NewStyle().
			Foreground(colorSubtle).
			Width(w).
			Render(fmt.Sprintf("v %d more below", remaining)))
	}

	windowedBody := strings.Join(windowed, "\n")

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render("↑↓: scroll • Esc: close")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		windowedBody,
		"",
		hint,
	)

	borderColor := colorSuccess
	switch {
	case len(m.validationDialog.errors) > 0:
		borderColor = colorDanger
	case len(m.validationDialog.warnings) > 0:
		borderColor = colorWarning
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

// renderSaveAsDialog renders the explicit "save to disk" prompt. It has
// two framings depending on how it was opened: a plain save (from
// Ctrl+S) versus a save-before-quit (from 'q' with unsaved changes),
// which also offers an explicit "quit without saving" path so the user
// isn't stuck if they don't want to save at all.
func (m Model) renderSaveAsDialog() string {
	w := dialogContentWidth(m.width)

	if m.saveAsDialog.confirmingOverwrite {
		return m.renderOverwriteConfirmDialog(w)
	}

	titleText := "Save docker-compose.yml"
	borderColor := colorAccent
	var prompt string
	var hint string

	if m.saveAsDialog.quitAfterSave {
		titleText = "You have unsaved changes"
		borderColor = colorWarning
		prompt = "Save before quitting? Enter a path, or quit without saving:"
		hint = "Enter: save & quit • q: quit without saving • Esc: keep working"
	} else {
		prompt = "Save to path:"
		hint = "Enter: save • Esc: cancel"
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Width(w).
		Render(titleText)

	promptLine := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render(prompt)

	input := m.saveAsDialog.pathInput.View()

	hintLine := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render(hint)

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		promptLine,
		input,
		"",
		hintLine,
	)

	return renderDialogBox(m.width, m.height, borderColor, content)
}

// renderOverwriteConfirmDialog renders the "file already exists"
// yes/no step of the save flow, in the same shape as the existing
// delete-service confirmation.
func (m Model) renderOverwriteConfirmDialog(w int) string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorWarning).
		Width(w).
		Render("File already exists")

	prompt := lipgloss.NewStyle().
		Width(w).
		Render(fmt.Sprintf("'%s' already exists. Overwrite it?", m.saveAsDialog.pendingPath))

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render("y: overwrite • n: pick a different path • Esc: cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		prompt,
		"",
		hint,
	)

	return renderDialogBox(m.width, m.height, colorWarning, content)
}
