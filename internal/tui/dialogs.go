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

// validationScrollWindow is the single source of truth for the
// validation dialog's scroll bounds: given a candidate scroll offset
// and the report's total row count, it returns the actual clamped
// offset, how many of those rows fit in the body's share of the dialog
// once indicators are accounted for, and whether each indicator is
// shown. renderValidationDialog uses it to build the visible window;
// updateValidation (model.go) uses it to clamp the *stored* scroll
// value on every keypress/wheel event, not just at render time —
// previously "down" had no upper bound at all (only the render-time
// window was clamped), so scrolling past the end of a long report and
// then scrolling back up required pressing "up" exactly as many times
// as "down" had been pressed past the end, even though the screen
// itself hadn't moved in the meantime.
func (m Model) validationScrollWindow(scroll, bodyLineCount int) (clamped, bodyBudget int, showAbove, showBelow bool) {
	// Fixed chrome around the body: border(2) + Padding(1,2)(2) +
	// title+blank(2) + blank+hint(2) = 8 rows not available to the body.
	const dialogChrome = 8
	visible := m.height - dialogChrome
	if visible < 3 {
		visible = 3
	}

	if scroll < 0 {
		scroll = 0
	}

	// Each shown indicator costs two rows, not one: the "N more
	// above/below" line itself plus a blank spacer that keeps it from
	// sitting flush against the report line right next to it (see
	// renderValidationDialog) — without the spacer it reads as just
	// another entry in the list rather than dialog chrome.
	const indicatorCost = 2

	showAbove = scroll > 0
	bodyBudget = visible
	if showAbove {
		bodyBudget -= indicatorCost
	}
	if bodyBudget < 1 {
		bodyBudget = 1
	}
	// Whether a "more below" indicator is needed depends on how many
	// lines are left after this window — which depends on bodyBudget,
	// which is why this has to be resolved after showAbove above.
	showBelow = bodyLineCount-scroll > bodyBudget
	if showBelow {
		bodyBudget -= indicatorCost
		if bodyBudget < 1 {
			bodyBudget = 1
		}
	}

	// Now that the final budget is known, clamp scroll so the window
	// doesn't run past the end of the report, and recheck whether that
	// still leaves anything above/below (clamping can change both).
	if maxScroll := bodyLineCount - bodyBudget; scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}
	showAbove = scroll > 0
	end := scroll + bodyBudget
	if end > bodyLineCount {
		end = bodyLineCount
	}
	showBelow = bodyLineCount-end > 0

	return scroll, bodyBudget, showAbove, showBelow
}

// validationDialogContentWidth is dialogContentWidth's preferred width,
// doubled, specifically for the validation report: unlike every other
// dialog (short prompts, single fields), it routinely carries long
// single-line messages — file paths, image names, JSON-pointer-style
// schema error locations — that wrap awkwardly and read cramped at the
// standard 60-column width every other dialog uses.
func validationDialogContentWidth(termWidth int) int {
	const preferred = 120
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

// buildValidationBodyLines renders the validation report body and also
// returns, for each warning (indexed the same as
// m.validationDialog.warnings), the line index within the returned
// slice where that warning's rendered block starts — used by
// updateValidation to scroll a newly tab-selected warning into view
// rather than leaving it to potentially render off-screen.
func (m Model) buildValidationBodyLines() ([]string, []int) {
	w := validationDialogContentWidth(m.width)

	var sections []string
	// lineCount tracks how many lines `sections` amounts to so far —
	// strings.Join(sections, "\n") then strings.Split(..., "\n") is
	// exactly len(sections) - 1 separator newlines plus each section's
	// own internal newlines (from lipgloss wrapping a long line), so
	// appending strings.Count(s, "\n")+1 per section keeps this in
	// sync with the real thing without re-joining/re-splitting on
	// every append just to count.
	lineCount := 0
	appendSection := func(s string) {
		sections = append(sections, s)
		lineCount += strings.Count(s, "\n") + 1
	}

	if len(m.validationDialog.errors) == 0 && len(m.validationDialog.warnings) == 0 && m.validationDialog.specValid {
		appendSection(lipgloss.NewStyle().
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
		appendSection(errHeader)
		for _, err := range m.validationDialog.errors {
			appendSection(lipgloss.NewStyle().
				Foreground(colorDanger).
				Width(w).
				Render("• " + err))
		}
	}

	var warningLineOffsets []int
	if len(m.validationDialog.warnings) > 0 {
		if len(sections) > 0 {
			appendSection("")
		}
		warnHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWarning).
			Width(w).
			Render(fmt.Sprintf("Warnings (%d) - valid, but worth a look:", len(m.validationDialog.warnings)))
		appendSection(warnHeader)

		warningLineOffsets = make([]int, len(m.validationDialog.warnings))
		for i, warning := range m.validationDialog.warnings {
			warningLineOffsets[i] = lineCount

			// Circle for a warning the picker can't do anything with,
			// arrow for one it can — so which warnings are actionable
			// is visible before you ever move the cursor onto them,
			// not just discovered by landing on one.
			fixable := m.validationDialog.secretRefs[i] != nil
			marker := "○ "
			color := colorWarning
			if fixable {
				marker = "▸ "
				color = colorFixable
			}
			style := lipgloss.NewStyle().Foreground(color).Width(w)
			if i == m.validationDialog.selectedWarning {
				style = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Width(w)
			}
			appendSection(style.Render(marker + warning))
		}
	}

	// Compose Specification conformance is its own section, checked
	// against the official schema (see ComposeConfig.ValidateAgainstSpec)
	// rather than komposer's own rules above — deliberately kept apart
	// so "komposer thinks this is risky" and "Docker Compose would
	// reject this outright" never get confused with one another.
	if len(sections) > 0 {
		appendSection("")
	}
	if m.validationDialog.specValid {
		appendSection(lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSuccess).
			Width(w).
			Render("✓ Compose specification: valid"))
	} else {
		specHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(colorDanger).
			Width(w).
			Render(fmt.Sprintf("✗ Compose specification (%d) - does not conform:", len(m.validationDialog.specIssues)))
		appendSection(specHeader)
		for _, issue := range m.validationDialog.specIssues {
			appendSection(lipgloss.NewStyle().
				Foreground(colorDanger).
				Width(w).
				Render("• " + issue))
		}
	}

	body := strings.Join(sections, "\n")
	return strings.Split(body, "\n"), warningLineOffsets
}

// clampedValidationScroll runs a candidate scroll offset through
// validationScrollWindow against the report's current line count and
// returns just the clamped value — used by updateValidation (model.go)
// to keep the stored offset itself always in range; see
// validationScrollWindow's doc comment for why that matters.
func (m Model) clampedValidationScroll(candidate int) int {
	bodyLines, _ := m.buildValidationBodyLines()
	clamped, _, _, _ := m.validationScrollWindow(candidate, len(bodyLines))
	return clamped
}

// ensureValidationLineVisible returns a scroll offset that brings the
// given body line (see buildValidationBodyLines's second return value)
// into view, scrolling up if it's above the current window and down if
// it's below — used after Tab/Shift+Tab moves the selected secret
// warning so it's never left rendered off-screen, requiring the user
// to separately scroll to find it.
func (m Model) ensureValidationLineVisible(line int) int {
	bodyLines, _ := m.buildValidationBodyLines()
	scroll := m.validationDialog.scroll
	_, bodyBudget, _, _ := m.validationScrollWindow(scroll, len(bodyLines))
	if bodyBudget <= 0 {
		return m.clampedValidationScroll(scroll)
	}
	if line < scroll {
		scroll = line
	} else if line >= scroll+bodyBudget {
		scroll = line - bodyBudget + 1
	}
	return m.clampedValidationScroll(scroll)
}

func (m Model) renderValidationDialog() string {
	w := validationDialogContentWidth(m.width)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Width(w).
		Render("Validation")

	bodyLines, _ := m.buildValidationBodyLines()

	// scroll is clamped to the exact same bounds updateValidation already
	// enforces on every keypress/wheel event (see validationScrollWindow
	// and setValidationScroll in model.go) — clamping again here too
	// means render never depends on the stored value already being
	// in-range, e.g. right after a shorter report replaces a longer one.
	scroll, bodyBudget, showAbove, showBelow := m.validationScrollWindow(m.validationDialog.scroll, len(bodyLines))
	end := scroll + bodyBudget
	if end > len(bodyLines) {
		end = len(bodyLines)
	}

	// Indicators are centered and set off with a rule so they read as
	// dialog chrome rather than another bullet in the list, and each
	// gets its own blank line toward the body so it never sits flush
	// against a warning/error line right next to it — validationScrollWindow
	// already reserves the extra row this costs out of bodyBudget.
	indicatorStyle := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Italic(true).
		Width(w).
		Align(lipgloss.Center)

	var windowed []string
	if showAbove {
		windowed = append(windowed, indicatorStyle.Render(fmt.Sprintf("── %d more above ──", scroll)))
		windowed = append(windowed, "")
	}
	windowed = append(windowed, bodyLines[scroll:end]...)
	if showBelow {
		windowed = append(windowed, "")
		windowed = append(windowed, indicatorStyle.Render(fmt.Sprintf("── %d more below ──", len(bodyLines)-end)))
	}

	windowedBody := strings.Join(windowed, "\n")

	var banner string
	if m.validationDialog.actionMessage != "" {
		bannerColor := colorSuccess
		prefix := "[OK] "
		if m.validationDialog.actionMessageErr {
			bannerColor = colorDanger
			prefix = "[ERROR] "
		}
		banner = lipgloss.NewStyle().
			Foreground(bannerColor).
			Width(w).
			Render(prefix+m.validationDialog.actionMessage) + "\n\n"
	}

	hintText := "↑↓: scroll • Esc: close"
	if len(m.validationDialog.warnings) > 0 {
		hintText = "↑↓: select • Enter: convert selected • PgUp/PgDn: scroll • Esc: close"
	}
	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render(hintText)

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		banner+windowedBody,
		"",
		hint,
	)

	borderColor := colorSuccess
	switch {
	case len(m.validationDialog.errors) > 0, !m.validationDialog.specValid:
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
		Render("Import compose.yaml")

	prompt := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render("Enter path to compose.yaml (or docker-compose.yml):")

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

	titleText := "Save compose.yaml"
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
