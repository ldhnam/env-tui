package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/ldhnam/envigator/internal/diff"
	"github.com/ldhnam/envigator/internal/gitguard"
	"github.com/ldhnam/envigator/internal/lint"
)

var (
	accent = lipgloss.Color("99")
	green  = lipgloss.Color("42")
	yellow = lipgloss.Color("220")
	red    = lipgloss.Color("196")
	gray   = lipgloss.Color("240")
	faint  = lipgloss.Color("250")

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder(), true).
			BorderForeground(gray).
			Padding(0, 1)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	curStyle   = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
	matchStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	diffStyle  = lipgloss.NewStyle().Foreground(yellow).Bold(true)
	missStyle  = lipgloss.NewStyle().Foreground(red).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(faint)
	dotStyle   = lipgloss.NewStyle().Foreground(accent)
)

// renderPane renders content inside a bordered panel of exactly h rows and w
// columns. Content is trimmed of trailing newlines and capped at h-2 lines so
// the final height is always deterministic (lipgloss counts a trailing newline
// as an extra line).
func renderPane(w, h int, content string) string {
	return panelStyle.Width(w).Height(h - 2).Render(truncateLines(strings.TrimRight(content, "\n"), h-2))
}

func (m Model) View() string {
	if m.showHelp {
		return m.helpView()
	}
	if m.workspaceView {
		return m.workspaceViewOverlay()
	}
	if m.matrixView {
		return m.matrixViewOverlay()
	}
	if m.searching {
		return m.searchView()
	}
	if m.passPrompting {
		return m.passPromptView()
	}
	if m.snapshotsView {
		return m.snapshotsViewOverlay()
	}
	if m.pushConfirm {
		return m.pushConfirmView()
	}
	if m.editing {
		return m.editorView()
	}
	if m.confirming {
		return m.confirmView()
	}
	if m.prompting {
		return m.promptView()
	}
	if len(m.files) == 0 || m.rep == nil {
		return m.emptyView()
	}

	w, _, mainH, colsH := m.layoutDims()
	missH := mainH - colsH
	if missH < 2 {
		missH = 2
	}

	cols := lipgloss.JoinHorizontal(lipgloss.Top,
		m.filesPane(colsH, m.filesW(w)),
		m.keysPane(colsH, m.keysW(w)),
		m.detailPane(colsH, m.detailW(w)),
	)
	var bottom string
	switch {
	case m.auditView:
		bottom = m.auditPane(missH, w)
	case m.lintView:
		bottom = m.lintPane(missH, w)
	default:
		bottom = m.missingPane(missH, w)
	}
	body := lipgloss.JoinVertical(lipgloss.Top, cols, bottom)
	return lipgloss.JoinVertical(lipgloss.Top, m.header(w), body, m.footer(w))
}

func (m Model) filesW(w int) int {
	fw := max(w/4, 20)
	return fw
}

func (m Model) keysW(w int) int {
	kw := max(w/4, 28)
	return kw
}

func (m Model) detailW(w int) int {
	return w - m.filesW(w) - m.keysW(w)
}

func (m Model) header(w int) string {
	title := fmt.Sprintf("envigator: %s", m.dir)
	var right string
	switch {
	case m.showSecrets:
		right = dotStyle.Render("● Secrets SHOWN [s]")
	case len(m.revealed) > 0:
		right = dotStyle.Render(fmt.Sprintf("● Stealth: %d revealed [s]", len(m.revealed)))
	case m.hoverKey != "":
		right = dotStyle.Render("● Stealth: hover reveal [s]")
	default:
		right = dotStyle.Render("● Stealth masked [s]")
	}
	right += "  " + m.gitBadge()
	right += "  " + m.vaultBadge()
	pad := max(w-lipgloss.Width(title)-lipgloss.Width(right), 1)
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(accent).
		Render(title + strings.Repeat(" ", pad) + right)
}

// gitBadge is the Safety Check status indicator shown in the header.
func (m Model) gitBadge() string {
	if m.git == nil || !m.git.IsRepo {
		return dimStyle.Render("git: n/a")
	}
	risks := 0
	for p, st := range m.git.ByPath {
		if isTemplateFile(p) {
			continue
		}
		if st == gitguard.Exposed || st == gitguard.Tracked {
			risks++
		}
	}
	if risks > 0 {
		return missStyle.Render(fmt.Sprintf("git: %d exposed", risks))
	}
	return matchStyle.Render("git: protected")
}

// vaultBadge reports the remote vault sync status in the header.
func (m Model) vaultBadge() string {
	if m.vaultProvider == "" {
		return ""
	}
	switch {
	case len(m.vaultSecrets) > 0:
		return matchStyle.Render(fmt.Sprintf("vault: %s · %d", m.vaultProvider, len(m.vaultSecrets)))
	case m.vaultErr != "":
		return missStyle.Render("vault: error")
	case m.vaultScan:
		return dimStyle.Render("vault: fetching…")
	default:
		return dimStyle.Render("vault: " + m.vaultProvider)
	}
}

// isTemplateFile reports whether a path is a committed template
// (.env.example) that is intentionally kept in the repository.
func isTemplateFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(filepath.Base(path)), ".example")
}

// gitFileMarker returns the per-file git safety glyph.
func (m Model) gitFileMarker(path string) string {
	if m.git == nil {
		return ""
	}
	if isTemplateFile(path) {
		return ""
	}
	switch m.git.ByPath[path] {
	case gitguard.Ignored:
		return " " + matchStyle.Render("✓")
	case gitguard.Tracked:
		return " " + diffStyle.Render("T")
	case gitguard.Exposed:
		return " " + missStyle.Render("!")
	default:
		return " " + dimStyle.Render("–")
	}
}

// filesPane renders the selectable list of discovered sources.
func (m Model) filesPane(h, w int) string {
	innerH := h - 2
	var rows []string
	for i, f := range m.files {
		mark := " "
		if i < len(m.selec) && m.selec[i] {
			mark = "x"
		}
		name := f.Label()
		if !f.Virtual && m.lint != nil {
			if n := len(m.lint.ByPath[f.Path]); n > 0 {
				name += " " + diffStyle.Render("⚠"+strconv.Itoa(n))
			}
		}
		if !f.Virtual {
			name += m.gitFileMarker(f.Path)
		}
		rows = append(rows, fmt.Sprintf("[%s] %s", mark, name))
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Target Files"))
	b.WriteString(dimStyle.Render("  [x select]"))
	b.WriteString("\n")
	start, end := visibleRange(len(rows), m.fileIdx, innerH-1)
	for i := start; i < end; i++ {
		line := dimStyle.Render(rows[i])
		if i == m.fileIdx && m.focus == panelFiles {
			line = curStyle.Render(rows[i])
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return renderPane(w, h, b.String())
}

// keysPane renders ghost keys and the union of environment keys with status
// glyphs and code-reference counts.
func (m Model) keysPane(h, w int) string {
	items := m.displayKeys()
	inv := m.inventoryKeys()
	innerH := h - 2
	lintCounts := m.lintCountByKey()
	secretKeys := m.secretKeySet()
	var rows []string
	for _, it := range items {
		row := m.keyGlyph(it) + " " + it.key
		if n := m.refCount(it.key); n > 0 {
			row += " " + dimStyle.Render("×"+strconv.Itoa(n))
		}
		if n := lintCounts[it.key]; n > 0 {
			row += diffStyle.Render(" ⚠" + strconv.Itoa(n))
		}
		if secretKeys[it.key] {
			row += missStyle.Render(" S")
		}
		if !it.ghost && m.audit != nil {
			if _, used := m.audit.Usages[it.key]; !used && inv[it.key] {
				row += dimStyle.Render(" z")
			}
		}
		rows = append(rows, truncate(row, w-4))
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("Keys"))
	b.WriteString(dimStyle.Render("  [j/k]"))
	b.WriteString("\n")
	start, end := visibleRange(len(rows), m.keyIdx, innerH-1)
	for i := start; i < end; i++ {
		line := rows[i]
		if i == m.keyIdx && m.focus == panelKeys {
			line = curStyle.Render(line)
		} else {
			line = dimStyle.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return renderPane(w, h, b.String())
}

func (m Model) refCount(key string) int {
	if m.audit == nil {
		return 0
	}
	if u := m.audit.Usages[key]; u != nil {
		return u.Count
	}
	return 0
}

func statusGlyph(st *diff.KeyState) string {
	switch st.Status {
	case diff.Match:
		return matchStyle.Render("✓")
	case diff.Diff:
		return diffStyle.Render("⚠")
	case diff.Missing:
		return missStyle.Render("✗")
	}
	return "?"
}
func (m Model) detailPane(h, w int) string {
	var b strings.Builder
	key := m.currentKey()
	if key == "" {
		b.WriteString(titleStyle.Render("Key"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("(no keys yet)"))
		return renderPane(w, h, b.String())
	}
	items := m.displayKeys()
	if m.keyIdx >= 0 && m.keyIdx < len(items) && items[m.keyIdx].ghost {
		return m.ghostDetailPane(h, w, key)
	}
	st := m.currentState()
	if st == nil {
		return renderPane(w, h, "")
	}

	b.WriteString(titleStyle.Render("Key: " + key))
	b.WriteString("\n")

	valueW := w - 22
	for i, f := range m.files {
		if i >= len(m.selec) || !m.selec[i] {
			continue
		}
		pres := st.Present[f.Path]
		name := truncate(f.Label(), 18)
		var line string
		switch {
		case !pres:
			line = fmt.Sprintf("%-19s: %s", name, missStyle.Render("(missing)"))
		case st.Values[f.Path] == "":
			line = fmt.Sprintf("%-19s: %s", name, dimStyle.Render("(empty)"))
		default:
			val := st.Values[f.Path]
			disp := val
			if !m.revealKey(key) {
				disp = maskVal()
			}
			line = fmt.Sprintf("%-19s: %s", name, dimStyle.Render(truncate(disp, valueW)))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	status := st.Status.String()
	var statusRendered string
	switch st.Status {
	case diff.Match:
		statusRendered = matchStyle.Render(status)
	case diff.Diff:
		statusRendered = diffStyle.Render(status)
	case diff.Missing:
		statusRendered = missStyle.Render(status)
	}
	b.WriteString("Status : " + statusRendered)
	if diff.FormatValid(st) {
		b.WriteString(lipgloss.NewStyle().Foreground(green).Render(" (Format Valid)"))
	}
	b.WriteString("\n")

	if len(m.rep.Files) > 1 && st.Status == diff.Diff {
		b.WriteString(dimStyle.Render("Values differ across selected sources."))
		b.WriteString("\n")
	}
	m.codeRefLine(key, &b)
	if n := m.lintCountByKey()[key]; n > 0 {
		b.WriteString(diffStyle.Render(fmt.Sprintf("Lint : %d issue(s) — press f", n)))
		b.WriteString("\n")
	}
	if matches := m.keySecrets(key); len(matches) > 0 {
		b.WriteString(missStyle.Render("Secret : " + joinPatterns(matches) + " detected"))
		b.WriteString("\n")
	}
	return renderPane(w, h, b.String())
}

// ghostDetailPane renders a key that is referenced in code but absent from
// every .env file.
func (m Model) ghostDetailPane(h, w int, key string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Key: " + key))
	b.WriteString("\n")
	b.WriteString(missStyle.Render("  ghost key — not present in any .env file"))
	b.WriteString("\n\n")
	m.codeRefLine(key, &b)
	return renderPane(w, h, b.String())
}

// codeRefLine renders how the focused key is referenced in the codebase.
func (m Model) codeRefLine(key string, b *strings.Builder) {
	if m.audit == nil {
		if m.auditScan {
			b.WriteString(dimStyle.Render("Code : scanning project…"))
		} else {
			b.WriteString(dimStyle.Render("Code : no audit data"))
		}
		b.WriteString("\n")
		return
	}
	u := m.audit.Usages[key]
	if u == nil {
		b.WriteString(dimStyle.Render("Code : no references in project"))
		b.WriteString("\n")
		return
	}
	names := fileNames(u.Files)
	if len(names) > 3 {
		names = names[:3]
		names = append(names, fmt.Sprintf("+%d more", len(u.Files)-3))
	}
	line := fmt.Sprintf("Code : %s (%s)",
		matchStyle.Render(strconv.Itoa(u.Count)+" ref"+plural(u.Count)),
		dimStyle.Render(strings.Join(names, ", ")))
	b.WriteString(line)
	b.WriteString("\n")
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func fileNames(files map[string][]int) []string {
	out := make([]string, 0, len(files))
	for f := range files {
		out = append(out, shortName(f))
	}
	sort.Strings(out)
	return out
}

// missingPane renders keys absent from the primary local file.
func (m Model) missingPane(h, w int) string {
	innerH := h - 2
	title := fmt.Sprintf("Missing Keys in %s", m.primaryLabel())
	var b strings.Builder
	b.WriteString(titleStyle.Render(title))
	if m.prim != "" {
		b.WriteString(fmt.Sprintf(" (%d):", len(m.rep.Missing)))
	}
	b.WriteString("\n")

	rows := make([]string, 0, len(m.rep.Missing))
	for _, st := range m.rep.Missing {
		sources := st.Sources(m.prim, m.rep.Files)
		names := make([]string, 0, len(sources))
		for _, s := range sources {
			names = append(names, shortName(s))
		}
		row := fmt.Sprintf("  %-26s Present in %s",
			st.Key, dimStyle.Render(strings.Join(names, ", ")))
		if n := m.refCount(st.Key); n > 0 {
			row += missStyle.Render(fmt.Sprintf("  [used ×%d]", n))
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		b.WriteString(dimStyle.Render("  everything in sync ✓"))
		b.WriteString("\n")
	}
	start, end := visibleRange(len(rows), m.missIdx, innerH-2)
	for i := start; i < end; i++ {
		line := rows[i]
		if i == m.missIdx && m.focus == panelMissing {
			line = curStyle.Render(strings.TrimSpace(rows[i]))
			b.WriteString(line)
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	if len(rows) > 0 {
		if m.focus == panelMissing && m.missIdx < len(rows) {
			b.WriteString(dimStyle.Render("  [a] autofill  [c] copy value"))
		} else {
			b.WriteString(dimStyle.Render("  [a] autofill  [c] copy value  (tab → missing)"))
		}
	}
	return renderPane(w, h, b.String())
}

// auditPane cross-references code usage with the environment inventory:
// ghost keys (used in code, missing from all .env files), keys used in code
// but missing from the primary file, and zombie keys (defined but unused).
func (m Model) auditPane(h, w int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Code Audit"))
	if m.audit != nil {
		b.WriteString(fmt.Sprintf("  (%d files · ghosts %d · zombies %d)",
			m.audit.Files, len(m.ghostKeys()), len(m.zombieKeys())))
	} else if m.auditScan {
		b.WriteString(dimStyle.Render("  (scanning project source files…)"))
	} else {
		b.WriteString(dimStyle.Render("  (no data)"))
	}
	b.WriteString(dimStyle.Render("  [v close]"))
	b.WriteString("\n")

	if m.audit == nil {
		if m.auditScan {
			b.WriteString(dimStyle.Render("  scanning .js .ts .py .go .rs .php …"))
			b.WriteString("\n")
		} else {
			b.WriteString(dimStyle.Render("  run 'r' to scan"))
			b.WriteString("\n")
		}
		return renderPane(w, h, b.String())
	}

	ghosts := m.ghostKeys()
	if len(ghosts) > 0 {
		b.WriteString(missStyle.Render("  Ghost Keys — used in code, missing from all .env files:"))
		b.WriteString("\n")
		for _, u := range ghosts {
			b.WriteString(fmt.Sprintf("    %-26s %d ref%s", u.Key, u.Count, plural(u.Count)))
			b.WriteString("\n")
		}
	}

	missUse := m.usedMissingPrimary()
	if len(missUse) > 0 {
		b.WriteString(diffStyle.Render("  Used but missing from " + m.primaryLabel() + ":"))
		b.WriteString("\n")
		for _, u := range missUse {
			b.WriteString(fmt.Sprintf("    %-26s %d ref%s", u.Key, u.Count, plural(u.Count)))
			b.WriteString("\n")
		}
	}

	zombies := m.zombieKeys()
	if len(zombies) > 0 {
		b.WriteString(dimStyle.Render("  Zombie Keys — in .env files, never referenced in code:"))
		b.WriteString("\n")
		for _, k := range zombies {
			b.WriteString(dimStyle.Render("    " + k))
			b.WriteString("\n")
		}
	}

	if len(ghosts) == 0 && len(missUse) == 0 && len(zombies) == 0 {
		b.WriteString(matchStyle.Render("  clean: every used var is defined, nothing unused"))
		b.WriteString("\n")
	}
	return renderPane(w, h, scrollLines(b.String(), m.paneScroll, h-2))
}

// lintCountByKey maps each key to the number of lint issues across all files.
func (m Model) lintCountByKey() map[string]int {
	out := make(map[string]int)
	if m.lint == nil {
		return out
	}
	for _, iss := range m.lint.Issues {
		if iss.Key != "" {
			out[iss.Key]++
		}
	}
	return out
}

// lintPane lists format & naming issues and detected credential leaks.
func (m Model) lintPane(h, w int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Format & Naming Lint"))
	if m.lint != nil {
		b.WriteString(fmt.Sprintf("  (%d files · %d issues · %d secrets)",
			m.lint.Files, len(m.lint.Issues), len(m.secretHits())))
	} else if m.lintScan {
		b.WriteString(dimStyle.Render("  (linting .env files…)"))
	} else {
		b.WriteString(dimStyle.Render("  (no data)"))
	}
	b.WriteString(dimStyle.Render("  [f close]"))
	b.WriteString("\n")

	if m.lint == nil {
		if m.lintScan {
			b.WriteString(dimStyle.Render("  checking UPPER_SNAKE_CASE, syntax, whitespace…"))
			b.WriteString("\n")
		} else {
			b.WriteString(dimStyle.Render("  run 'r' to scan"))
			b.WriteString("\n")
		}
		return renderPane(w, h, b.String())
	}
	hits := m.secretHits()
	if len(hits) > 0 {
		b.WriteString(missStyle.Render("  Secrets Detected — values match known credential formats:"))
		b.WriteString("\n")
		for _, h := range hits {
			b.WriteString(fmt.Sprintf("    %-20s %-14s %s",
				truncate(h.Key, 20), h.Pattern, dimStyle.Render(shortName(h.Path))))
			b.WriteString("\n")
		}
	}
	if len(m.lint.Issues) == 0 {
		b.WriteString(matchStyle.Render("  clean: keys well-formed, no whitespace or syntax issues"))
		b.WriteString("\n")
		return renderPane(w, h, b.String())
	}

	paths := make([]string, 0, len(m.lint.ByPath))
	for p := range m.lint.ByPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		b.WriteString(dimStyle.Render(shortName(p)))
		b.WriteString("\n")
		for _, iss := range m.lint.ByPath[p] {
			key := iss.Key
			if key == "" {
				key = "-"
			}
			line := fmt.Sprintf("  L%-3d %-18s %-11s %s",
				iss.Line, truncate(key, 18), m.kindStyle(iss.Kind), dimStyle.Render(iss.Detail))
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return renderPane(w, h, scrollLines(b.String(), m.paneScroll, h-2))
}

func (m Model) kindStyle(k lint.Kind) string {
	switch k {
	case lint.BadName, lint.Syntax:
		return missStyle.Render(string(k))
	case lint.Whitespace, lint.Duplicate:
		return diffStyle.Render(string(k))
	default:
		return dimStyle.Render(string(k))
	}
}

func (m Model) footer(w int) string {
	hint := "j/k nav · tab focus · s secrets · space reveal · e edit · a autofill · x select · c value · C name · E export · B block · T shell · R shell · P pull · U push · t template · S snapshot · / search · v audit · f lint · ? help · q quit"
	if m.toast != "" && time.Since(m.toastAt) < toastDur {
		hint = dotStyle.Render("• ") + m.toast
	}
	return lipgloss.NewStyle().Width(w).Foreground(faint).Render(truncate(hint, w))
}

func (m Model) emptyView() string {
	msg := fmt.Sprintf("No .env files found in %s", m.dir)
	box := panelStyle.Width(46).Padding(1).Render(
		titleStyle.Render("envigator") + "\n\n" + dimStyle.Render(msg) + "\n\n" + dimStyle.Render("q to quit"),
	)
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

func (m Model) promptView() string {
	box := panelStyle.Width(52).Render(
		titleStyle.Render("Autofill "+m.promptKey) + "\n\n" +
			dimStyle.Render("Value (leave empty for placeholder):") + "\n" +
			m.input.View() + "\n\n" +
			dimStyle.Render("enter save · esc cancel"),
	)
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

// editorView is the multi-line In-Place Editor overlay for complex values.
func (m Model) editorView() string {
	lines := []string{
		titleStyle.Render("In-Place Editor — " + m.editKey),
		"",
		dimStyle.Render("Value (multi-line supported, e.g. PEM keys / JSON):"),
		m.editor.View(),
		"",
		dimStyle.Render("ctrl+s save · esc cancel"),
	}
	box := panelStyle.Width(min(m.width-4, 84)).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

// workspaceViewOverlay lists monorepo/workspace contexts with .env files.
func (m Model) workspaceViewOverlay() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Workspaces"))
	b.WriteString(dimStyle.Render("  [W close · j/k navigate · enter switch]"))
	b.WriteString("\n\n")
	if len(m.workspaces) == 0 {
		b.WriteString(dimStyle.Render("  no nested .env workspaces found"))
	} else {
		for i, ws := range m.workspaces {
			label := ws.path
			if ws.marker != "" {
				label += "  [" + ws.marker + "]"
			}
			label += fmt.Sprintf("  (%d .env)", ws.envs)
			line := "  " + label
			if i == m.wsIdx {
				line = curStyle.Render(line)
			} else {
				line = dimStyle.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  enter switch context · esc close"))
	box := panelStyle.Width(min(m.width-4, 76)).Render(b.String())
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

// matrixViewOverlay is a grid of selected profiles (columns) x keys (rows),
// for spotting missing variables across environments at a glance.
func (m Model) matrixViewOverlay() string {
	files := m.selectedFiles()
	var b strings.Builder
	b.WriteString(titleStyle.Render("Profile Matrix"))
	b.WriteString(dimStyle.Render("  [M close · j/k rows · h/l cols]"))
	b.WriteString("\n")

	if len(files) == 0 || m.rep == nil {
		b.WriteString(dimStyle.Render("  nothing to show — select some source files"))
		box := panelStyle.Width(50).Render(b.String())
		return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
	}

	keyW := 20
	colW := (m.width - keyW - 8) / len(files)
	if colW < 5 {
		colW = 5
	}
	header := strings.Repeat(" ", keyW) + " "
	for _, f := range files {
		header += "| " + truncate(shortName(f.Path), colW) + " "
	}
	b.WriteString(dimStyle.Render(header))
	b.WriteString("\n")

	for i, st := range m.rep.All {
		key := truncate(st.Key, keyW)
		row := key + strings.Repeat(" ", keyW-lipgloss.Width(key)) + " "
		for _, f := range files {
			cell := dimStyle.Render("—")
			if v, ok := st.Values[f.Path]; ok {
				disp := maskVal()
				if m.revealKey(st.Key) {
					disp = v
				}
				cell = truncate(disp, colW)
			}
			row += "| " + cell + " "
		}
		if i == m.matrixRow {
			b.WriteString(curStyle.Render(row))
		} else {
			b.WriteString(row)
		}
		b.WriteString("\n")
	}
	box := panelStyle.Width(min(m.width-2, 120)).Render(b.String())
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

// searchView is the fuzzy search overlay for filtering keys by name/value.
func (m Model) searchView() string {
	items := m.displayKeys()
	var b strings.Builder
	b.WriteString(titleStyle.Render("Fuzzy Search"))
	b.WriteString("\n\n")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	if len(m.searchResults) == 0 {
		b.WriteString(dimStyle.Render("  no matches"))
	} else {
		maxResults := 10
		for n := 0; n < len(m.searchResults) && n < maxResults; n++ {
			idx := m.searchResults[n]
			key := items[idx].key
			val, _ := m.valueFor(key)
			if !m.revealKey(key) {
				val = maskVal()
			}
			line := fmt.Sprintf("  %-24s %s", truncate(key, 24), truncate(val, 28))
			if n == m.searchIdx {
				line = curStyle.Render(line)
			} else {
				line = dimStyle.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		if len(m.searchResults) > maxResults {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  … %d more", len(m.searchResults)-maxResults)))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  enter open · j/k navigate · esc close"))
	box := panelStyle.Width(66).Render(b.String())
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

// passPromptView asks for the snapshot passphrase (masked input).
func (m Model) passPromptView() string {
	action := "create a snapshot"
	if m.passAction == "restore" {
		action = "restore " + m.passSnapshot
	}
	box := panelStyle.Width(56).Render(
		titleStyle.Render("Encrypted Snapshots") + "\n\n" +
			dimStyle.Render("Passphrase to "+action+":") + "\n" +
			m.input.View() + "\n\n" +
			dimStyle.Render("enter confirm · esc cancel"),
	)
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

// snapshotsViewOverlay lists local encrypted snapshots with actions.
func (m Model) snapshotsViewOverlay() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Encrypted Snapshots"))
	b.WriteString("\n\n")
	if len(m.snapshots) == 0 {
		b.WriteString(dimStyle.Render("  no snapshots yet — press c to create one"))
		b.WriteString("\n\n")
	} else {
		for i, s := range m.snapshots {
			line := "  " + s
			if i == m.snapIdx {
				line = curStyle.Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(dimStyle.Render("  c create · enter restore · d delete · esc close"))
	box := panelStyle.Width(64).Render(b.String())
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

// pushConfirmView is the confirmation gate before pushing to a vault.
func (m Model) pushConfirmView() string {
	n := 0
	if f := m.fileFor(m.prim); f != nil {
		n = len(f.Values)
	}
	lines := []string{
		titleStyle.Render("Remote Vault Sync"),
		"",
		fmt.Sprintf("Push %s %d secret(s) to %s?",
			missStyle.Render(m.primaryLabel()), n, missStyle.Render(m.vaultProvider)),
		"",
		missStyle.Render("This writes to the remote secret manager and cannot be undone."),
		"",
		dimStyle.Render("[y] push    [n] cancel"),
	}
	box := panelStyle.Width(60).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

// confirmView is the Pre-Commit Guard gate shown before a secret-like value
// is written to the primary .env file.
func (m Model) confirmView() string {
	lines := []string{
		titleStyle.Render("Pre-Commit Guard"),
		"",
		fmt.Sprintf("The value for %s looks like a real credential:", dimStyle.Render(m.confirmKey)),
		"",
	}
	for _, mt := range m.confirmMatches {
		lines = append(lines, fmt.Sprintf("  %s %s",
			missStyle.Render("●"),
			missStyle.Render(mt.Name)+dimStyle.Render(" ("+maskSecret(mt.Raw)+")"),
		))
	}
	lines = append(lines,
		"",
		missStyle.Render("Saving this could expose an unencrypted secret if committed."),
		"",
		dimStyle.Render("[y] save anyway    [n] cancel"),
	)
	box := panelStyle.Width(66).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

func maskSecret(s string) string {
	if len(s) <= 6 {
		return "••••"
	}
	return s[:4] + "…" + strings.Repeat("•", 4)
}

func (m Model) helpView() string {
	lines := []string{
		titleStyle.Render("envigator keybindings"),
		"",
		"  j/k, ↑/↓   move in focused panel",
		"  tab, ←/→   cycle focus: files → keys → missing",
		"  s          toggle secret obfuscation",
		"  space      reveal the focused key's values",
		"  mouse      hover a key to reveal it (select on hover)",
		"  x          toggle include source file",
		"  a          autofill selected missing key into primary .env",
		"  e          edit the focused key in place (multi-line)",
		"  c / C      copy key value / key name",
		"  E          copy export KEY=\"VALUE\" line for the focused key",
		"  B          copy export block for all keys in the primary .env",
		"  T          cycle export shell format (bash / zsh / fish)",
		"  v          toggle Code Audit (ghost / zombie / used-but-missing)",
		"  f          toggle Format & Naming Lint + leak detector",
		"  r / R      reload & rescan / spawn a nested shell with the loaded env",
		"  P / U      pull vault secrets into primary .env / push primary to vault",
		"  t          generate a sanitized .env.example template",
		"  S          encrypted snapshots (create / restore / delete)",
		"  M          profile matrix (all files x all keys)",
		"  W          monorepo workspace switcher",
		"  g / G      jump to top / bottom",
		"  ? / /      toggle help / fuzzy search keys & values",
		"  q          quit",
		"",
		dimStyle.Render("Values are masked by default; press s to reveal all, space to reveal the"),
		dimStyle.Render("focused key, or hover a key with the mouse."),
		"",
		dimStyle.Render("Autofill is guarded: values that match Stripe / AWS / OpenAI / GitHub token"),
		dimStyle.Render("patterns require a y/n confirmation before being written to .env."),
		"",
		dimStyle.Render("Safety Check: the header git badge shows whether .env files are git-ignored."),
		dimStyle.Render("In Target Files: ✓ ignored · ! not ignored · T tracked · – not a git repo."),
		"",
		dimStyle.Render("In-Place Editor (e): type multi-line values (RSA keys, JSON); ctrl+s saves,"),
		dimStyle.Render("esc cancels. Multi-line values are stored as quoted \\n-escaped strings."),
		dimStyle.Render("Code Audit scans .js .ts .py .go .rs .php .rb .sh and more for env var usage."),
		dimStyle.Render("Press q or ? to close."),
	}
	box := panelStyle.Width(46).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Align(lipgloss.Center, lipgloss.Center).Render(box)
}

// --- helpers ---

func visibleRange(total, cursor, max int) (int, int) {
	if total <= 0 || max <= 0 {
		return 0, 0
	}
	if total <= max {
		return 0, total
	}
	start := cursor - max/2
	if start < 0 {
		start = 0
	}
	if start+max > total {
		start = total - max
	}
	return start, start + max
}

func truncate(s string, max int) string {
	if max <= 1 {
		return "…"
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	var out strings.Builder
	width := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if width+rw > max-1 {
			break
		}
		out.WriteString(string(r))
		width += rw
	}
	return out.String() + "…"
}

func truncateLines(s string, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[:max], "\n")
}

// scrollLines returns up to max lines from content, starting at offset.
func scrollLines(s string, offset, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if offset > len(lines) {
		offset = len(lines)
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + max
	if end > len(lines) {
		end = len(lines)
	}
	if offset >= end {
		return ""
	}
	return strings.Join(lines[offset:end], "\n")
}

func maskVal() string {
	return "••••••••"
}

func shortName(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}
