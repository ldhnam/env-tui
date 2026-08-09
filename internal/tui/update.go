package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ldhnam/envigator/internal/diff"
	"github.com/ldhnam/envigator/internal/envfile"
	"github.com/ldhnam/envigator/internal/secrets"
	"github.com/ldhnam/envigator/internal/snapshot"
	"github.com/ldhnam/envigator/internal/vault"
)

// vaultMsg is delivered when a vault fetch completes.
type vaultMsg struct {
	secrets map[string]string
	err     error
}

// vaultPushMsg is delivered when a vault push completes.
type vaultPushMsg struct {
	err error
}

func vaultFetchCmd(m Model) tea.Cmd {
	return func() tea.Msg {
		secrets, err := vault.Fetch(vault.Provider(m.vaultProvider), m.vaultProject, m.vaultEnv)
		return vaultMsg{secrets: secrets, err: err}
	}
}

func vaultPushCmd(m Model, secrets map[string]string) tea.Cmd {
	return func() tea.Msg {
		err := vault.Push(vault.Provider(m.vaultProvider), m.vaultProject, m.vaultEnv, secrets)
		return vaultPushMsg{err: err}
	}
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, auditCmd(m.dir), lintCmd(m.dir))
	if m.vaultProvider != "" {
		cmds = append(cmds, vaultFetchCmd(m))
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		if m.prompting {
			return m.updatePrompt(msg)
		}
		if m.confirming {
			return m.updateConfirm(msg)
		}
		if m.editing {
			return m.updateEditor(msg)
		}
		if m.pushConfirm {
			return m.updatePushConfirm(msg)
		}
		if m.passPrompting {
			return m.updatePassPrompt(msg)
		}
		if m.snapshotsView {
			return m.updateSnapshots(msg)
		}
		if m.searching {
			return m.updateSearch(msg)
		}
		if m.matrixView {
			return m.updateMatrix(msg)
		}
		if m.workspaceView {
			return m.updateWorkspaces(msg)
		}
		return m.handleKey(msg)
	case toastClearMsg:
		m.toast = ""
		return m, nil
	case auditMsg:
		m.auditScan = false
		if msg.err != nil {
			m.toastf("audit scan failed: %v", msg.err)
			return m, toastCmd()
		}
		m.audit = msg.rep
		return m, nil
	case lintMsg:
		m.lintScan = false
		if msg.err != nil {
			m.toastf("lint scan failed: %v", msg.err)
			return m, toastCmd()
		}
		m.lint = msg.rep
		return m, nil
	case shellDoneMsg:
		if msg.err != nil {
			m.toastf("nested shell exited with error: %v", msg.err)
		} else {
			m.toastf("nested shell exited")
		}
		return m, toastCmd()
	case vaultMsg:
		m.vaultScan = false
		if msg.err != nil {
			m.vaultErr = msg.err.Error()
			m.toastf("vault sync failed: %v", msg.err)
			return m, toastCmd()
		}
		m.vaultErr = ""
		m.vaultSecrets = msg.secrets
		m.reload()
		m.toastf("loaded %d secrets from %s", len(msg.secrets), m.vaultProvider)
		return m, toastCmd()
	case vaultPushMsg:
		if msg.err != nil {
			m.toastf("push to %s failed: %v", m.vaultProvider, msg.err)
		} else {
			m.toastf("pushed %d secrets to %s", len(m.vaultSecrets), m.vaultProvider)
		}
		return m, toastCmd()
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
	case " ":
		if key := m.currentKey(); key != "" {
			if m.revealed[key] {
				delete(m.revealed, key)
			} else {
				m.revealed[key] = true
			}
		}
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
		return m.copyValue()
	case "C":
		return m.copyKeyName()
	case "E":
		return m.copyExport()
	case "B":
		return m.copyExportBlock()
	case "T":
		m.cycleShell()
		return m, nil
	case "e":
		return m.openEditor()
	case "v":
		m.auditView = !m.auditView
		m.paneScroll = 0
		if m.auditView {
			m.lintView = false
		}
		return m, nil
	case "f":
		m.lintView = !m.lintView
		m.paneScroll = 0
		if m.lintView {
			m.auditView = false
		}
		return m, nil
	case "r":
		m.reload()
		m.audit = nil
		m.auditScan = true
		m.lint = nil
		m.lintScan = true
		m.toastf("reloaded %d file(s), rescanning…", len(m.files))
		return m, tea.Batch(auditCmd(m.dir), lintCmd(m.dir), toastCmd())
	case "R":
		m.toastf("spawning nested %s with %d env vars", m.shell, len(m.loadedEnv()))
		return m, tea.Batch(m.spawnShell(), toastCmd())
	case "P":
		return m.vaultPull()
	case "U":
		return m.vaultPushPrompt()
	case "t":
		return m.generateTemplate()
	case "S":
		return m.openSnapshots()
	case "?":
		m.showHelp = !m.showHelp
		return m, nil
	case "/":
		return m.openSearch()
	case "M":
		return m.openMatrix()
	case "W":
		return m.openWorkspaces()
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

// handleMouse implements hover-to-reveal and wheel scrolling.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	ev := tea.MouseEvent(msg)
	if ev.IsWheel() {
		switch ev.Button {
		case tea.MouseButtonWheelUp:
			return m.scrollWheel(-1)
		case tea.MouseButtonWheelDown:
			return m.scrollWheel(1)
		}
		return m, nil
	}
	switch ev.Action {
	case tea.MouseActionMotion:
		key, idx := m.hoverTarget(ev.X, ev.Y)
		m.hoverKey = key
		if idx >= 0 {
			m.keyIdx = idx
		}
	case tea.MouseActionRelease:
		m.hoverKey = ""
	}
	return m, nil
}

// scrollWheel scrolls the focused list or a read-only panel.
func (m Model) scrollWheel(d int) (tea.Model, tea.Cmd) {
	if m.lintView || m.auditView {
		m.paneScroll += d
		if m.paneScroll < 0 {
			m.paneScroll = 0
		}
		return m, nil
	}
	switch m.focus {
	case panelFiles:
		m.fileIdx = clamp(m.fileIdx+d, 0, len(m.files)-1)
	case panelKeys:
		if m.rep != nil {
			m.keyIdx = clamp(m.keyIdx+d, 0, len(m.displayKeys())-1)
		}
	case panelMissing:
		if m.rep != nil && !m.bottomView() {
			m.missIdx = clamp(m.missIdx+d, 0, len(m.rep.Missing)-1)
		}
	}
	return m, nil
}

// hoverTarget maps a terminal cell (0-based) to the key under the cursor and
// its index in the display keys list (-1 if none).
func (m Model) hoverTarget(x, y int) (string, int) {
	w, _, _, colsH := m.layoutDims()
	fw := m.filesW(w)
	kw := m.keysW(w)
	items := m.displayKeys()

	start, end := visibleRange(len(items), m.keyIdx, colsH-3)
	if y >= 3 && y < 3+(end-start) && x >= fw+1 && x < fw+kw-1 {
		idx := start + (y - 3)
		if idx >= 0 && idx < len(items) {
			return items[idx].key, idx
		}
		return "", -1
	}
	if x >= fw+kw+1 && y >= 3 && y < colsH {
		return m.currentKey(), m.keyIdx
	}
	return "", -1
}

func (m *Model) move(d int) {
	switch m.focus {
	case panelFiles:
		m.fileIdx = clamp(m.fileIdx+d, 0, len(m.files)-1)
	case panelKeys:
		if m.rep != nil {
			m.keyIdx = clamp(m.keyIdx+d, 0, len(m.displayKeys())-1)
		}
	case panelMissing:
		if m.rep == nil || m.bottomView() {
			return
		}
		m.missIdx = clamp(m.missIdx+d, 0, len(m.rep.Missing)-1)
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
	m.keyIdx = len(m.displayKeys()) - 1
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
	if m.bottomView() {
		m.toastf("switch back to Missing Keys (v/f) to autofill")
		return m, toastCmd()
	}
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

// copyValue copies the selected key's value (from primary, else first source)
// to the system clipboard.
func (m Model) copyValue() (tea.Model, tea.Cmd) {
	if m.bottomView() {
		m.toastf("switch back to Missing Keys (v/f) to copy")
		return m, toastCmd()
	}
	key := m.currentKey()
	val, ok := m.valueFor(key)
	if !ok {
		m.toastf("no value to copy for %s", key)
		return m, toastCmd()
	}
	if err := clipboard.WriteAll(val); err != nil {
		m.toastf("copy failed: %v", err)
		return m, toastCmd()
	}
	m.toastf("copied %s value to clipboard", key)
	return m, toastCmd()
}

// copyKeyName copies just the selected key's name.
func (m Model) copyKeyName() (tea.Model, tea.Cmd) {
	if m.bottomView() {
		m.toastf("switch back to Missing Keys (v/f) to copy")
		return m, toastCmd()
	}
	key := m.currentKey()
	if key == "" {
		return m, nil
	}
	if err := clipboard.WriteAll(key); err != nil {
		m.toastf("copy failed: %v", err)
		return m, toastCmd()
	}
	m.toastf("copied key name %s", key)
	return m, toastCmd()
}

// copyExport copies an `export KEY="VALUE"` line for the focused key,
// formatted for the current shell target.
func (m Model) copyExport() (tea.Model, tea.Cmd) {
	if m.bottomView() {
		m.toastf("switch back to Missing Keys (v/f) to copy")
		return m, toastCmd()
	}
	key := m.currentKey()
	line, ok := m.exportLine(key)
	if !ok {
		m.toastf("no value to export for %s", key)
		return m, toastCmd()
	}
	if err := clipboard.WriteAll(line); err != nil {
		m.toastf("copy failed: %v", err)
		return m, toastCmd()
	}
	m.toastf("copied export %s (%s)", key, m.shell)
	return m, toastCmd()
}

// copyExportBlock copies export lines for every key in the primary file.
func (m Model) copyExportBlock() (tea.Model, tea.Cmd) {
	if m.bottomView() {
		m.toastf("switch back to Missing Keys (v/f) to copy")
		return m, toastCmd()
	}
	block := m.exportBlock()
	if block == "" {
		m.toastf("nothing to export")
		return m, toastCmd()
	}
	if err := clipboard.WriteAll(block); err != nil {
		m.toastf("copy failed: %v", err)
		return m, toastCmd()
	}
	m.toastf("copied %d exports to clipboard (%s)", strings.Count(block, "\n")+1, m.shell)
	return m, toastCmd()
}

// cycleShell advances the clipboard export target shell.
func (m *Model) cycleShell() {
	for i, s := range shellNames {
		if s == m.shell {
			m.shell = shellNames[(i+1)%len(shellNames)]
			return
		}
	}
	m.shell = shellNames[0]
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
		if matches := secrets.Detect(val); len(matches) > 0 {
			m.confirming = true
			m.confirmKey = key
			m.confirmVal = val
			m.confirmMatches = matches
			return m, nil
		}
		return m.saveConfirmed(key, val, false)
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

// saveConfirmed writes key=val to the primary file. flagged indicates the
// value matched a secret pattern and the user chose to save anyway.
func (m Model) saveConfirmed(key, val string, flagged bool) (tea.Model, tea.Cmd) {
	if err := m.appendPrimary(key, val); err != nil {
		m.toastf("error writing %s: %v", m.primaryLabel(), err)
	} else if flagged {
		m.toastf("added %s to %s (contains secret-like value)", key, m.primaryLabel())
	} else {
		m.toastf("added %s to %s", key, m.primaryLabel())
	}
	m.reload()
	return m, toastCmd()
}

// updateConfirm handles the y/n decision for a secret-like autofill value.
func (m Model) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		key, val := m.confirmKey, m.confirmVal
		replace := m.confirmReplace
		m.confirming = false
		m.confirmKey, m.confirmVal, m.confirmMatches = "", "", nil
		m.confirmReplace = false
		if replace {
			return m.doWrite(key, val, true)
		}
		return m.saveConfirmed(key, val, true)
	case "n", "N", "esc":
		key := m.confirmKey
		m.confirming = false
		m.confirmKey, m.confirmVal, m.confirmMatches = "", "", nil
		m.confirmReplace = false
		m.toastf("cancelled — %s not saved (secret-like value)", key)
		return m, toastCmd()
	}
	return m, nil
}

// openEditor starts the multi-line in-place editor for the focused key,
// pre-filled with the key's current value in the primary file.
func (m Model) openEditor() (tea.Model, tea.Cmd) {
	if m.bottomView() {
		m.toastf("switch back to Missing Keys (v/f) to edit")
		return m, toastCmd()
	}
	key := m.currentKey()
	if key == "" {
		return m, nil
	}
	val := ""
	if f := m.fileFor(m.prim); f != nil {
		val = f.Values[key]
	}
	m.editKey = key
	m.editing = true
	m.editor = textarea.New()
	m.editor.SetValue(val)
	m.editor.Placeholder = "value for " + key + " (multi-line ok)"
	m.editor.CharLimit = 0
	m.editor.ShowLineNumbers = false
	if m.width > 30 {
		m.editor.SetWidth(m.width - 24)
	}
	m.editor.SetHeight(8)
	m.editor.Focus()
	return m, nil
}

func (m Model) updateEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+s":
		return m.saveEditor()
	case "esc":
		m.editing = false
		m.editor.Blur()
		m.toastf("edit cancelled — %s unchanged", m.editKey)
		return m, toastCmd()
	default:
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	}
}

// saveEditor writes the edited value back in place, gated by the secret guard.
func (m Model) saveEditor() (tea.Model, tea.Cmd) {
	key, val := m.editKey, m.editor.Value()
	m.editing = false
	m.editor.Blur()
	if key == "" {
		return m, nil
	}
	if matches := secrets.Detect(val); len(matches) > 0 {
		m.confirming = true
		m.confirmKey = key
		m.confirmVal = val
		m.confirmMatches = matches
		m.confirmReplace = true
		return m, nil
	}
	return m.doWrite(key, val, false)
}

// doWrite persists key=val to the primary file in place. flagged indicates a
// secret was confirmed by the user.
func (m Model) doWrite(key, val string, flagged bool) (tea.Model, tea.Cmd) {
	if err := m.setPrimaryValue(key, val); err != nil {
		m.toastf("error writing %s: %v", m.primaryLabel(), err)
	} else if flagged {
		m.toastf("updated %s in %s (contains secret-like value)", key, m.primaryLabel())
	} else {
		m.toastf("updated %s in %s", key, m.primaryLabel())
	}
	m.reload()
	return m, toastCmd()
}

// fileFor returns the parsed envfile for a path, if loaded.
func (m Model) fileFor(path string) *envfile.File {
	for _, f := range m.files {
		if f.Path == path {
			return f
		}
	}
	return nil
}

// vaultPull copies vault secrets missing from the primary file into it.
func (m Model) vaultPull() (tea.Model, tea.Cmd) {
	if m.vaultProvider == "" {
		m.toastf("no vault configured (see -vault flag)")
		return m, toastCmd()
	}
	if len(m.vaultSecrets) == 0 {
		if m.vaultScan {
			m.toastf("vault %s still fetching…", m.vaultProvider)
		} else if m.vaultErr != "" {
			m.toastf("vault unavailable: %s", m.vaultErr)
		} else {
			m.toastf("vault %s has no secrets", m.vaultProvider)
		}
		return m, toastCmd()
	}
	primary := make(map[string]bool)
	if f := m.fileFor(m.prim); f != nil {
		for k := range f.Values {
			primary[k] = true
		}
	}
	added := 0
	for _, k := range sortedStringKeys(m.vaultSecrets) {
		if primary[k] {
			continue
		}
		if err := m.appendPrimary(k, m.vaultSecrets[k]); err == nil {
			added++
		}
	}
	if added > 0 {
		m.reload()
	}
	m.toastf("pulled %d secret(s) from %s into %s", added, m.vaultProvider, m.primaryLabel())
	return m, toastCmd()
}

// vaultPushPrompt opens a confirmation gate before pushing to the provider.
func (m Model) vaultPushPrompt() (tea.Model, tea.Cmd) {
	if m.vaultProvider == "" {
		m.toastf("no vault configured (see -vault flag)")
		return m, toastCmd()
	}
	if f := m.fileFor(m.prim); f == nil || len(f.Values) == 0 {
		m.toastf("nothing to push")
		return m, toastCmd()
	}
	m.pushConfirm = true
	return m, nil
}

func (m Model) updatePushConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		m.pushConfirm = false
		secrets := make(map[string]string)
		if f := m.fileFor(m.prim); f != nil {
			for k := range f.Values {
				secrets[k] = f.Values[k]
			}
		}
		return m, vaultPushCmd(m, secrets)
	case "n", "N", "esc":
		m.pushConfirm = false
		m.toastf("push to %s cancelled", m.vaultProvider)
		return m, toastCmd()
	}
	return m, nil
}

func sortedStringKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// openSnapshots shows the encrypted snapshots panel.
func (m Model) openSnapshots() (tea.Model, tea.Cmd) {
	list, _ := snapshot.List(m.dir)
	m.snapshots = list
	m.snapIdx = 0
	m.snapshotsView = true
	return m, nil
}

func (m Model) updateSnapshots(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.snapshotsView = false
		return m, nil
	case "c":
		m.startPassPrompt("create", "")
		return m, nil
	case "j", "down":
		m.snapIdx = clamp(m.snapIdx+1, 0, len(m.snapshots)-1)
		return m, nil
	case "k", "up":
		m.snapIdx = clamp(m.snapIdx-1, 0, len(m.snapshots)-1)
		return m, nil
	case "enter":
		if len(m.snapshots) == 0 {
			return m, nil
		}
		m.startPassPrompt("restore", m.snapshots[m.snapIdx])
		return m, nil
	case "d":
		if len(m.snapshots) == 0 {
			return m, nil
		}
		name := m.snapshots[m.snapIdx]
		if err := snapshot.Delete(m.dir, name); err != nil {
			m.toastf("delete failed: %v", err)
			return m, toastCmd()
		}
		m.snapshots = append(m.snapshots[:m.snapIdx], m.snapshots[m.snapIdx+1:]...)
		m.snapIdx = clamp(m.snapIdx, 0, len(m.snapshots)-1)
		m.toastf("deleted snapshot %s", name)
		return m, toastCmd()
	}
	return m, nil
}

func (m *Model) startPassPrompt(action, snap string) {
	m.passAction = action
	m.passSnapshot = snap
	m.snapshotsView = false
	m.input.SetValue("")
	m.input.Placeholder = "passphrase"
	m.input.EchoMode = textinput.EchoPassword
	m.input.Focus()
	m.passPrompting = true
}

func (m Model) updatePassPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		pass := m.input.Value()
		m.passPrompting = false
		m.input.Blur()
		m.input.EchoMode = textinput.EchoNormal
		if pass == "" {
			m.toastf("passphrase required")
			return m, toastCmd()
		}
		return m.runPassAction(pass)
	case "esc":
		m.passPrompting = false
		m.input.Blur()
		m.input.EchoMode = textinput.EchoNormal
		m.toastf("snapshot action cancelled")
		return m, toastCmd()
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
}

func (m Model) runPassAction(pass string) (tea.Model, tea.Cmd) {
	switch m.passAction {
	case "create":
		files := make(map[string]string)
		for _, f := range m.files {
			if f.Virtual {
				continue
			}
			if data, err := os.ReadFile(f.Path); err == nil {
				files[f.Name] = string(data)
			}
		}
		name, err := snapshot.Create(m.dir, pass, files)
		if err != nil {
			m.toastf("snapshot failed: %v", err)
		} else {
			m.toastf("created snapshot %s (%d files)", name, len(files))
		}
		if list, lerr := snapshot.List(m.dir); lerr == nil {
			m.snapshots = list
		}
	case "restore":
		files, err := snapshot.Read(m.dir, m.passSnapshot, pass)
		if err != nil {
			m.toastf("restore failed: %v", err)
			m.passAction, m.passSnapshot = "", ""
			m.snapshotsView = true
			return m, toastCmd()
		}
		n := 0
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(m.dir, name), []byte(content), 0o644); err == nil {
				n++
			}
		}
		m.toastf("restored %d file(s) from %s", n, m.passSnapshot)
		m.reload()
	default:
		m.toastf("unknown snapshot action %q", m.passAction)
	}
	m.passAction, m.passSnapshot = "", ""
	m.snapshotsView = true
	return m, toastCmd()
}

// openSearch starts the fuzzy key/value search overlay.
func (m Model) openSearch() (tea.Model, tea.Cmd) {
	m.searching = true
	m.searchResults = m.computeSearch("")
	m.searchIdx = 0
	m.input.SetValue("")
	m.input.Placeholder = "fuzzy search keys / values…"
	m.input.EchoMode = textinput.EchoNormal
	m.input.Focus()
	return m, nil
}

func (m Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searching = false
		m.input.Blur()
		return m, nil
	case "enter":
		if len(m.searchResults) > 0 {
			m.keyIdx = m.searchResults[m.searchIdx]
			m.focus = panelKeys
		}
		m.searching = false
		m.input.Blur()
		m.toastf("")
		return m, nil
	case "j", "down":
		if len(m.searchResults) > 0 {
			m.searchIdx = clamp(m.searchIdx+1, 0, len(m.searchResults)-1)
		}
		return m, nil
	case "k", "up":
		if len(m.searchResults) > 0 {
			m.searchIdx = clamp(m.searchIdx-1, 0, len(m.searchResults)-1)
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.searchResults = m.computeSearch(m.input.Value())
		m.searchIdx = 0
		return m, cmd
	}
}

func (m Model) openMatrix() (tea.Model, tea.Cmd) {
	m.matrixView = true
	m.matrixRow = clamp(m.keyIdx, 0, len(m.rep.All)-1)
	m.matrixCol = 0
	return m, nil
}

func (m Model) updateMatrix(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sel := m.selectedFiles()
	switch msg.String() {
	case "esc", "M", "q":
		m.matrixView = false
		return m, nil
	case "j", "down":
		m.matrixRow = clamp(m.matrixRow+1, 0, len(m.rep.All)-1)
	case "k", "up":
		m.matrixRow = clamp(m.matrixRow-1, 0, len(m.rep.All)-1)
	case "h", "left":
		m.matrixCol = clamp(m.matrixCol-1, 0, len(sel)-1)
	case "l", "right":
		m.matrixCol = clamp(m.matrixCol+1, 0, len(sel)-1)
	}
	return m, nil
}

func (m Model) openWorkspaces() (tea.Model, tea.Cmd) {
	m.workspaces = findWorkspaces(m.dir, 3)
	m.wsIdx = 0
	m.workspaceView = true
	return m, nil
}

func (m Model) updateWorkspaces(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "W", "q":
		m.workspaceView = false
		return m, nil
	case "j", "down":
		m.wsIdx = clamp(m.wsIdx+1, 0, len(m.workspaces)-1)
		return m, nil
	case "k", "up":
		m.wsIdx = clamp(m.wsIdx-1, 0, len(m.workspaces)-1)
		return m, nil
	case "enter":
		if m.wsIdx >= 0 && m.wsIdx < len(m.workspaces) {
			return m.switchWorkspace(m.workspaces[m.wsIdx].path)
		}
		return m, nil
	}
	return m, nil
}

// switchWorkspace points the model at a different directory and rescans.
func (m Model) switchWorkspace(path string) (tea.Model, tea.Cmd) {
	m.dir = path
	m.workspaceView = false
	m.reload()
	m.audit = nil
	m.auditScan = true
	m.lint = nil
	m.lintScan = true
	if m.vaultProvider != "" {
		m.vaultSecrets = nil
		m.vaultErr = ""
		m.vaultScan = true
	}
	cmds := []tea.Cmd{auditCmd(m.dir), lintCmd(m.dir), toastCmd()}
	if m.vaultProvider != "" {
		cmds = append(cmds, vaultFetchCmd(m))
	}
	m.toastf("workspace: %s", path)
	return m, tea.Batch(cmds...)
}
