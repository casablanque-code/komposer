package tui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) saveFile() tea.Cmd {
	return func() tea.Msg {
		filename := "docker-compose.yml"
		yamlBytes, err := m.config.ExportYAML()
		if err != nil {
			return saveResult{filename: filename, err: err}
		}

		if err := os.WriteFile(filename, yamlBytes, 0644); err != nil {
			return saveResult{filename: filename, err: err}
		}

		return saveResult{filename: filename, err: nil}
	}
}

type clearSaveBannerMsg struct{}

func clearSaveBanner() tea.Msg {
	return clearSaveBannerMsg{}
}
