package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var shellNames = []string{"bash", "zsh", "fish"}

// detectShell guesses the user's shell from $SHELL, defaulting to bash.
func detectShell() string {
	base := filepath.Base(os.Getenv("SHELL"))
	switch base {
	case "fish":
		return "fish"
	case "zsh":
		return "zsh"
	default:
		return "bash"
	}
}

// shellExport renders one environment assignment for the target shell.
// Bash and zsh share `export KEY="value"`; fish uses `set -gx`.
func shellExport(shell, key, value string) string {
	quoted := quoteShellValue(value, shell)
	if shell == "fish" {
		return fmt.Sprintf("set -gx %s %s", key, quoted)
	}
	return fmt.Sprintf("export %s=%s", key, quoted)
}

// quoteShellValue wraps a value in double quotes, escaping shell
// metacharacters for the target shell.
func quoteShellValue(v, shell string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '$':
			b.WriteString(`\$`)
		case '`':
			if shell == "fish" {
				b.WriteRune(r)
			} else {
				b.WriteString("\\`")
			}
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// exportLine renders the export assignment for a key, using the primary
// file's value (falling back to the first selected source).
func (m Model) exportLine(key string) (string, bool) {
	val, ok := m.valueFor(key)
	if !ok {
		return "", false
	}
	return shellExport(m.shell, key, val), true
}

// exportBlock renders an export assignment for every key in the primary
// file, ready to be sourced.
func (m Model) exportBlock() string {
	var lines []string
	if f := m.fileFor(m.prim); f != nil {
		for _, k := range f.Keys {
			lines = append(lines, shellExport(m.shell, k, f.Values[k]))
		}
	}
	return strings.Join(lines, "\n")
}

// valueFor resolves a key's value from the primary file, falling back to
// the first selected source that defines it.
func (m Model) valueFor(key string) (string, bool) {
	if m.rep == nil {
		return "", false
	}
	st := m.rep.ByKey[key]
	if st == nil {
		return "", false
	}
	if v, ok := st.Values[m.prim]; ok {
		return v, true
	}
	for _, f := range m.rep.Files {
		if st.Present[f] {
			return st.Values[f], true
		}
	}
	return "", false
}
