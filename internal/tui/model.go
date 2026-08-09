package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"env-tui/internal/audit"
	"env-tui/internal/diff"
	"env-tui/internal/envfile"
)

type panel int

const (
	panelFiles panel = iota
	panelKeys
	panelMissing
)

func (p panel) String() string {
	switch p {
	case panelFiles:
		return "files"
	case panelKeys:
		return "keys"
	case panelMissing:
		return "missing"
	}
	return "unknown"
}

type toastClearMsg struct{}

const toastDur = 3 * time.Second

type auditMsg struct {
	rep *audit.Report
	err error
}

func auditCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		rep, err := audit.Scan(dir)
		return auditMsg{rep: rep, err: err}
	}
}

type Model struct {
	dir   string
	files []*envfile.File
	selec []bool
	prim  string
	rep   *diff.Report

	focus   panel
	fileIdx int
	keyIdx  int
	missIdx int

	showSecrets bool
	showHelp    bool

	audit     *audit.Report
	auditView bool
	auditScan bool // scan in flight

	toast   string
	toastAt time.Time

	prompting bool
	promptKey string
	input     textinput.Model

	width  int
	height int
}

func New(dir string) Model {
	m := Model{dir: dir, auditScan: true}
	m.input = textinput.New()
	m.input.Placeholder = "value"
	m.input.CharLimit = 4096
	m.reload()
	return m
}

func (m *Model) reload() {
	paths, err := envfile.Discover(m.dir)
	if err != nil {
		m.toastf("error: %v", err)
		return
	}
	envs := make(map[string]*envfile.File, len(paths))
	m.files = m.files[:0]
	for _, p := range paths {
		f, perr := envfile.Parse(p, envfile.IsRemote(p))
		if perr != nil {
			f = &envfile.File{Path: p, Name: filepath.Base(p), Values: make(map[string]string)}
		}
		m.files = append(m.files, f)
		envs[p] = f
	}
	if len(m.selec) != len(m.files) {
		m.selec = make([]bool, len(m.files))
		for i := range m.selec {
			m.selec[i] = true
		}
	}
	selected := m.selectedPaths()
	m.prim = envfile.Primary(selected)
	m.rep = diff.Build(m.prim, selected, envs)
	m.clampCursors()
}

func (m *Model) selectedPaths() []string {
	var out []string
	for i, f := range m.files {
		if i < len(m.selec) && m.selec[i] {
			out = append(out, f.Path)
		}
	}
	return out
}

func (m *Model) toggleFile() {
	if m.fileIdx >= 0 && m.fileIdx < len(m.selec) {
		m.selec[m.fileIdx] = !m.selec[m.fileIdx]
	}
	m.reload()
}

func (m *Model) primaryLabel() string {
	if m.prim == "" {
		return ""
	}
	return filepath.Base(m.prim)
}

func (m *Model) currentKey() string {
	if m.rep == nil {
		return ""
	}
	switch m.focus {
	case panelMissing:
		if m.missIdx >= 0 && m.missIdx < len(m.rep.Missing) {
			return m.rep.Missing[m.missIdx].Key
		}
	case panelFiles, panelKeys:
		items := m.displayKeys()
		if m.keyIdx >= 0 && m.keyIdx < len(items) {
			return items[m.keyIdx].key
		}
	}
	return ""
}

func (m *Model) currentState() *diff.KeyState {
	if m.rep == nil {
		return nil
	}
	if m.focus == panelMissing {
		if m.missIdx >= 0 && m.missIdx < len(m.rep.Missing) {
			return m.rep.ByKey[m.rep.Missing[m.missIdx].Key]
		}
		return nil
	}
	items := m.displayKeys()
	if m.keyIdx >= 0 && m.keyIdx < len(items) && !items[m.keyIdx].ghost {
		return m.rep.ByKey[items[m.keyIdx].key]
	}
	return nil
}

func (m *Model) clampCursors() {
	if m.fileIdx < 0 {
		m.fileIdx = 0
	}
	if m.fileIdx >= len(m.files) {
		m.fileIdx = len(m.files) - 1
	}
	if m.keyIdx < 0 {
		m.keyIdx = 0
	}
	if m.rep != nil {
		if n := len(m.displayKeys()); m.keyIdx >= n {
			m.keyIdx = n - 1
		}
	}
	if m.missIdx < 0 {
		m.missIdx = 0
	}
	if m.rep != nil && m.missIdx >= len(m.rep.Missing) {
		m.missIdx = len(m.rep.Missing) - 1
	}
}

func (m *Model) toastf(format string, args ...any) {
	m.toast = fmt.Sprintf(format, args...)
	m.toastAt = time.Now()
}

func toastCmd() tea.Cmd {
	return tea.Tick(toastDur, func(time.Time) tea.Msg { return toastClearMsg{} })
}

func (m *Model) appendPrimary(key, value string) error {
	f, err := os.OpenFile(m.prim, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n%s=%s\n", key, value)
	return err
}
