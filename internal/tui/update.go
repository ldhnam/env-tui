package tui

import (
	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"env-tui/internal/diff"
)

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.prompting {
			return m.updatePrompt(msg)
		}
		return m.handleKey(msg)
	case toastClearMsg:
		m.toast = ""
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "s":
		m.showSecrets = !m.showSecrets
		return m, nil
	case "tab", "right", "l":
		m.focus = nextPanel(m.focus)
		return m, nil
	case "left", "h":
		m.focus = prevPanel(m.focus)
		return m, nil
	case "up", "k":
		m.move(-1)
		return m, nil
	case "down", "j":
		m.move(1)
		return m, nil
	case "g":
		m.jumpToTop()
		return m, nil
	case "G", "end":
		m.jumpToBottom()
		return m, nil
	case "x":
		if m.focus == panelFiles {
			m.toggleFile()
		}
		return m, nil
	case "a":
		return m.autofill()
	case "c":
		return m.copyKey()
	case "r":
		m.reload()
		m.toastf("reloaded %d file(s)", len(m.files))
		return m, toastCmd()
	case "?", "/":
		m.showHelp = !m.showHelp
		return m, nil
	}
	return m, nil
}

func nextPanel(p panel) panel {
	if p == panelMissing {
		return panelFiles
	}
	return p + 1
}

func prevPanel(p panel) panel {
	if p == panelFiles {
		return panelMissing
	}
	return p - 1
}

func (m *Model) move(d int) {
	switch m.focus {
	case panelFiles:
		m.fileIdx = clamp(m.fileIdx+d, 0, len(m.files)-1)
	case panelKeys:
		if m.rep != nil {
			m.keyIdx = clamp(m.keyIdx+d, 0, len(m.rep.All)-1)
		}
	case panelMissing:
		if m.rep != nil {
			m.missIdx = clamp(m.missIdx+d, 0, len(m.rep.Missing)-1)
		}
	}
}

func (m *Model) jumpToTop() {
	m.fileIdx, m.keyIdx, m.missIdx = 0, 0, 0
}

func (m *Model) jumpToBottom() {
	if m.rep == nil {
		return
	}
	m.fileIdx = len(m.files) - 1
	m.keyIdx = len(m.rep.All) - 1
	m.missIdx = len(m.rep.Missing) - 1
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// autofill opens the value prompt for the selected missing key.
func (m Model) autofill() (tea.Model, tea.Cmd) {
	st := m.currentState()
	if st == nil {
		return m, nil
	}
	if st.Status != diff.Missing {
		m.toastf("%s is not missing in %s", st.Key, m.primaryLabel())
		return m, toastCmd()
	}
	m.promptKey = st.Key
	m.input.SetValue("")
	m.input.Placeholder = "value for " + st.Key
	m.input.Focus()
	m.prompting = true
	return m, nil
}

// copyKey copies the selected key's value (from primary, else first source)
// to the system clipboard.
func (m Model) copyKey() (tea.Model, tea.Cmd) {
	st := m.currentState()
	if st == nil {
		return m, nil
	}
	val, ok := st.Values[m.prim]
	if !ok {
		for _, f := range m.rep.Files {
			if st.Present[f] {
				val = st.Values[f]
				ok = true
				break
			}
		}
	}
	if !ok {
		m.toastf("no value to copy for %s", st.Key)
		return m, toastCmd()
	}
	if err := clipboard.WriteAll(val); err != nil {
		m.toastf("copy failed: %v", err)
		return m, toastCmd()
	}
	m.toastf("copied %s to clipboard", st.Key)
	return m, toastCmd()
}

func (m Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := m.input.Value()
		key := m.promptKey
		m.prompting = false
		m.input.Blur()
		if key == "" {
			return m, nil
		}
		if err := m.appendPrimary(key, val); err != nil {
			m.toastf("error writing %s: %v", m.primaryLabel(), err)
		} else {
			m.toastf("added %s to %s", key, m.primaryLabel())
		}
		m.reload()
		return m, toastCmd()
	case "esc":
		m.prompting = false
		m.input.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}
