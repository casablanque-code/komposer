package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// convertSecretCmd applies the chosen strategy (0 = .env, 1 = Compose
// secret) to the given service/key and returns the resulting
// secretActionResult as a tea.Msg, the same async-command pattern
// saveFileAs uses for Ctrl+S. Disk writes happen here rather than in
// pkg/composer, which otherwise has no disk side effects of its own —
// ConvertSecretToEnvFile/ConvertSecretToComposeSecret only mutate the
// in-memory config and hand back what needs writing.
func (m *Model) convertSecretCmd(service, key string, strategy int) tea.Cmd {
	return func() tea.Msg {
		switch strategy {
		case 0: // .env
			line, err := m.config.ConvertSecretToEnvFile(service, key)
			if err != nil {
				return secretActionResult{err: err}
			}
			if err := appendEnvFileLine(".env", line); err != nil {
				return secretActionResult{err: fmt.Errorf("updating .env: %w", err)}
			}
			return secretActionResult{message: fmt.Sprintf(
				"Moved %s to .env — make sure .env is in your .gitignore.", key)}

		case 1: // Compose secret
			secretName, path, value, err := m.config.ConvertSecretToComposeSecret(service, key)
			if err != nil {
				return secretActionResult{err: err}
			}
			if err := writeSecretFile(path, value); err != nil {
				return secretActionResult{err: fmt.Errorf("writing %s: %w", path, err)}
			}
			return secretActionResult{message: fmt.Sprintf(
				"Moved %s to a Compose secret ('%s', %s) — the service now reads it "+
					"from /run/secrets/%s instead of the environment; make sure %s is "+
					"in your .gitignore.", key, secretName, path, secretName, path)}
		}
		return secretActionResult{}
	}
}

// appendEnvFileLine appends a "KEY=value" line to a .env file,
// creating it if it doesn't exist yet. If the file already defines
// that key (e.g. this conversion was already applied once), it's left
// untouched rather than duplicated.
func appendEnvFileLine(path, line string) error {
	key := line
	if idx := strings.Index(line, "="); idx >= 0 {
		key = line[:idx]
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, l := range strings.Split(string(existing), "\n") {
		l = strings.TrimSpace(l)
		if idx := strings.Index(l, "="); idx >= 0 && l[:idx] == key {
			return nil // already present — nothing to do
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	// A leading newline before the new line is harmless when the file
	// is empty/new (yields one blank line at the top, which .env
	// parsers ignore) and keeps a from getting glued onto an existing
	// last line that didn't end in "\n".
	_, err = f.WriteString(line + "\n")
	return err
}

// writeSecretFile writes value to path (e.g. "./secrets/db_password.txt"),
// creating any missing parent directories. Secret files are written
// with 0600 rather than the compose file's usual 0644 — unlike the
// compose file itself, this one directly contains the sensitive value.
func writeSecretFile(path, value string) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(value), 0600)
}

// renderSecretStrategyDialog renders the "how should this secret be
// stored" picker opened from the validation dialog.
func (m Model) renderSecretStrategyDialog() string {
	w := dialogContentWidth(m.width)

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorTitle).
		Width(w).
		Render(fmt.Sprintf("Secret: %s (%s)", m.secretStrategy.key, m.secretStrategy.service))

	kind := "hardcoded value"
	if m.secretStrategy.empty {
		kind = "no value set"
	}
	subtitle := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render("Currently: " + kind + ". How should this be stored instead?")

	options := []struct {
		label string
		desc  string
	}{
		{"Convert to .env", "Substituted as ${" + m.secretStrategy.key + "}, value moved to a .env file"},
		{"Convert to Compose secret", "Removed from environment:, delivered as a file under /run/secrets/ instead"},
		{"Keep as is", "Leave it exactly as it is now"},
	}

	var lines []string
	lines = append(lines, title, "", subtitle, "")
	for i, opt := range options {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(colorSubtle).Width(w)
		if i == m.secretStrategy.selected {
			prefix = "▸ "
			style = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Width(w)
		}
		lines = append(lines, style.Render(prefix+opt.label))
		descStyle := lipgloss.NewStyle().Foreground(colorSubtle).Width(w)
		lines = append(lines, descStyle.Render("    "+opt.desc))
	}

	hint := lipgloss.NewStyle().
		Foreground(colorSubtle).
		Width(w).
		Render("↑↓: choose • Enter: confirm • Esc: back")
	lines = append(lines, "", hint)

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)

	return renderDialogBox(m.width, m.height, colorAccent, content)
}
