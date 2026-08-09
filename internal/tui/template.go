package tui

import (
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ldhnam/envigator/internal/envfile"
)

// templateNeedsPlaceholder reports whether a value should be masked in a
// generated template. Kept as a thin wrapper for testing.
func templateNeedsPlaceholder(key, val string) bool {
	return envfile.NeedsPlaceholder(key, val)
}

// generateTemplate writes a sanitized .env.example from the primary .env,
// replacing secret-looking values with <YOUR_KEY> placeholders while
// preserving comments.
func (m Model) generateTemplate() (tea.Model, tea.Cmd) {
	if m.prim == "" {
		m.toastf("no primary .env to template")
		return m, toastCmd()
	}
	content, err := envfile.BuildExampleTemplate(m.dir)
	if err != nil {
		m.toastf("template failed: %v", err)
		return m, toastCmd()
	}
	path := filepath.Join(m.dir, ".env.example")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		m.toastf("failed to write .env.example: %v", err)
		return m, toastCmd()
	}
	m.reload()
	m.toastf("generated .env.example")
	return m, toastCmd()
}
