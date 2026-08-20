package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/casablanque-code/komposer/pkg/composer"
)

func (m Model) updatePresetPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.presetPicker.stage == 0 {
		switch msg.String() {
		case "esc":
			m.currentMode = modeNormal
			return m, nil

		case "up", "k":
			if m.presetPicker.selected > 0 {
				m.presetPicker.selected--
			}
			return m, nil

		case "down", "j":
			if m.presetPicker.selected < len(composer.Presets)-1 {
				m.presetPicker.selected++
			}
			return m, nil

		case "enter":
			m.presetPicker.stage = 1
			m.presetPicker.chosenPreset = m.presetPicker.selected
			m.presetPicker.nameInput.Focus()
			return m, nil
		}
	} else {
		switch msg.String() {
		case "esc":
			m.presetPicker.stage = 0
			return m, nil

		case "enter":
			name := m.presetPicker.nameInput.Value()
			if name != "" && m.config.GetService(name) == nil {
				preset := composer.Presets[m.presetPicker.chosenPreset]
				m.config.AddPreset(preset, name)
				m.selected = len(m.config.Services) - 1
				m.dirty = true
			}
			m.currentMode = modeNormal
			return m, nil
		}

		m.presetPicker.nameInput, cmd = m.presetPicker.nameInput.Update(msg)
		return m, cmd
	}

	return m, nil
}
