package tui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// saveFileAs writes the current config to the given path. quitAfterSave
// carries through to the resulting saveResult so the Update loop knows
// whether to exit the program after a successful write (used when this
// was triggered by 'q' with unsaved changes) or just show the "Saved"
// confirmation and return to the normal view (Ctrl+S).
func (m *Model) saveFileAs(path string, quitAfterSave bool) tea.Cmd {
	return func() tea.Msg {
		yamlBytes, err := m.config.ExportYAML()
		if err != nil {
			return saveResult{filename: path, err: err, quitAfterSave: quitAfterSave}
		}

		if err := os.WriteFile(path, yamlBytes, 0644); err != nil {
			return saveResult{filename: path, err: err, quitAfterSave: quitAfterSave}
		}

		return saveResult{filename: path, err: nil, quitAfterSave: quitAfterSave}
	}
}

type clearSaveBannerMsg struct{}

func clearSaveBanner() tea.Msg {
	return clearSaveBannerMsg{}
}
