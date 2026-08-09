package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ldhnam/envigator/internal/audit"
	"github.com/ldhnam/envigator/internal/diff"
	"github.com/ldhnam/envigator/internal/envfile"
	"github.com/ldhnam/envigator/internal/gitguard"
	"github.com/ldhnam/envigator/internal/lint"
	"github.com/ldhnam/envigator/internal/secrets"
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

type lintMsg struct {
	rep *lint.Report
	err error
}

func lintCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		rep, err := lint.Scan(dir)
		return lintMsg{rep: rep, err: err}
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

	// Stealth/masking: values are masked unless globally shown (showSecrets),
	// individually revealed (revealed), or hovered (hoverKey).
	showSecrets bool
	showHelp    bool
	revealed    map[string]bool
	hoverKey    string

	audit     *audit.Report
	auditView bool
	auditScan bool // scan in flight

	lint     *lint.Report
	lintView bool
	lintScan bool // scan in flight

	git *gitguard.Report // git-ignore safety status for discovered files

	toast   string
	toastAt time.Time

	prompting bool
	promptKey string
	input     textinput.Model

	// Pre-commit guard: an entered value that looks like a secret must be
	// confirmed before it is written to the primary .env file.
	confirming     bool
	confirmKey     string
	confirmVal     string
	confirmMatches []secrets.Match
	confirmReplace bool // true when the guarded write is an in-place edit

	// In-place editor: multi-line text editing for complex key values.
	editing bool
	editKey string
	editor  textarea.Model

	// Clipboard export target shell: bash, zsh, or fish.
	shell string

	// Remote vault sync (secret manager integration).
	vaultProvider string
	vaultProject  string
	vaultEnv      string
	vaultSecrets  map[string]string
	vaultScan     bool
	vaultErr      string
	pushConfirm   bool

	// Encrypted snapshots.
	snapshotsView bool
	snapshots     []string // newest first
	snapIdx       int
	passPrompting bool
	passAction    string // "create" / "restore"
	passSnapshot  string

	// Fuzzy search.
	searching     bool
	searchResults []int
	searchIdx     int

	// Scroll offset for read-only panels (lint/audit).
	paneScroll int

	width  int
	height int
}

// Options configures optional behavior passed to New.
type Options struct {
	VaultProvider string
	VaultProject  string
	VaultEnv      string
}

func New(dir string, opts ...Options) Model {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	m := Model{
		dir:           dir,
		auditScan:     true,
		lintScan:      true,
		revealed:      make(map[string]bool),
		shell:         detectShell(),
		vaultProvider: o.VaultProvider,
		vaultProject:  o.VaultProject,
		vaultEnv:      o.VaultEnv,
	}
	if m.vaultProvider != "" {
		m.vaultScan = true
	}
	m.input = textinput.New()
	m.input.Placeholder = "value"
	m.input.CharLimit = 4096
	m.reload()
	return m
}

// bottomView reports whether the bottom panel is showing audit or lint
// (i.e. the missing-keys panel is not active).
func (m Model) bottomView() bool {
	return m.auditView || m.lintView
}

// revealKey reports whether key's values should be shown unmasked.
func (m Model) revealKey(key string) bool {
	if m.showSecrets {
		return true
	}
	return key != "" && (m.hoverKey == key || m.revealed[key])
}

// layoutDims returns the effective terminal dimensions and the top-columns
// height, shared by rendering and mouse hit-testing.
func (m Model) layoutDims() (w, h, mainH, colsH int) {
	h = max(m.height, 24)
	w = max(m.width, 80)
	mainH = h - 2
	colsH = max(mainH*3/5, 5)
	return w, h, mainH, colsH
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
	if vf := m.vaultFile(); vf != nil {
		m.files = append(m.files, vf)
		envs[vf.Path] = vf
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
	m.git = gitguard.Check(m.dir, paths)
	m.clampCursors()
}

// vaultFile returns the in-memory remote source for a loaded secret-manager
// vault, or nil when none is loaded.
func (m Model) vaultFile() *envfile.File {
	if len(m.vaultSecrets) == 0 {
		return nil
	}
	f := &envfile.File{
		Path:    "vault:" + m.vaultProvider,
		Name:    m.vaultProvider + " (vault)",
		Remote:  true,
		Virtual: true,
		Values:  make(map[string]string, len(m.vaultSecrets)),
	}
	keys := make([]string, 0, len(m.vaultSecrets))
	for k := range m.vaultSecrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		f.Add(k, m.vaultSecrets[k])
	}
	return f
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
	_, err = fmt.Fprintf(f, "\n%s=%s\n", key, envfile.QuoteValue(value))
	return err
}

// setPrimaryValue writes key=value in place: it replaces the key's first
// assignment in the primary file, or appends it when the key is absent.
func (m *Model) setPrimaryValue(key, value string) error {
	data, err := os.ReadFile(m.prim)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	replaced := false
	prefix := key + "="
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "export ") {
			t = strings.TrimPrefix(t, "export ")
		}
		if strings.HasPrefix(t, prefix) && strings.Index(t, "=") == len(key) {
			lines[i] = key + "=" + envfile.QuoteValue(value)
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, key+"="+envfile.QuoteValue(value))
	}
	return os.WriteFile(m.prim, []byte(strings.Join(lines, "\n")), 0o644)
}
