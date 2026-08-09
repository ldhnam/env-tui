package tui

import (
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// shellDoneMsg is delivered after the nested shell session exits.
type shellDoneMsg struct {
	err error
}

// spawnShell launches an interactive nested shell (R) populated with the
// loaded environment variables. The variables are applied only to the child
// process via cmd.Env — the global/system environment is never modified.
func (m Model) spawnShell() tea.Cmd {
	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		shellPath = "/bin/sh"
	}
	cmd := exec.Command(shellPath, "-i")
	cmd.Env = m.childEnv()
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return shellDoneMsg{err: err}
	})
}

// loadedEnv returns the environment variables loaded from the primary file,
// with gaps filled from the other discovered files.
func (m Model) loadedEnv() map[string]string {
	env := make(map[string]string)
	if f := m.fileFor(m.prim); f != nil {
		for _, k := range f.Keys {
			env[k] = f.Values[k]
		}
	}
	for _, f := range m.files {
		if f.Path == m.prim {
			continue
		}
		for _, k := range f.Keys {
			if _, ok := env[k]; !ok {
				env[k] = f.Values[k]
			}
		}
	}
	return env
}

// childEnv builds the environment for the nested shell: the parent's current
// environment with the loaded variables applied on top.
func (m Model) childEnv() []string {
	loaded := m.loadedEnv()
	env := make([]string, 0, len(os.Environ())+len(loaded))
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.Index(kv, "="); i >= 0 {
			key = kv[:i]
		}
		if _, ok := loaded[key]; ok {
			continue // overridden below with the loaded value
		}
		env = append(env, kv)
	}
	for k, v := range loaded {
		env = append(env, k+"="+v)
	}
	return env
}
