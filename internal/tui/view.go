package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"env-tui/internal/diff"
	"env-tui/internal/lint"
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
	title := fmt.Sprintf("env-tui: %s", m.dir)
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
	pad := max(w-lipgloss.Width(title)-lipgloss.Width(right), 1)
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(accent).
		Render(title + strings.Repeat(" ", pad) + right)
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
		if m.lint != nil {
			if n := len(m.lint.ByPath[f.Path]); n > 0 {
				name += " " + diffStyle.Render("⚠"+strconv.Itoa(n))
			}
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
	var rows []string
	for _, it := range items {
		row := m.keyGlyph(it) + " " + it.key
		if n := m.refCount(it.key); n > 0 {
			row += " " + dimStyle.Render("×"+strconv.Itoa(n))
		}
		if n := lintCounts[it.key]; n > 0 {
			row += diffStyle.Render(" ⚠" + strconv.Itoa(n))
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
	return renderPane(w, h, b.String())
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

// lintPane lists format & naming issues per file.
func (m Model) lintPane(h, w int) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Format & Naming Lint"))
	if m.lint != nil {
		b.WriteString(fmt.Sprintf("  (%d files · %d issues)", m.lint.Files, len(m.lint.Issues)))
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
	return renderPane(w, h, b.String())
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
	hint := "j/k nav · tab focus · s secrets · space reveal · x select · a autofill · c copy · v audit · f lint · r reload · ? help · q quit"
	if m.toast != "" && time.Since(m.toastAt) < toastDur {
		hint = dotStyle.Render("• ") + m.toast
	}
	return lipgloss.NewStyle().Width(w).Foreground(faint).Render(truncate(hint, w))
}

func (m Model) emptyView() string {
	msg := fmt.Sprintf("No .env files found in %s", m.dir)
	box := panelStyle.Width(46).Padding(1).Render(
		titleStyle.Render("env-tui") + "\n\n" + dimStyle.Render(msg) + "\n\n" + dimStyle.Render("q to quit"),
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

func (m Model) helpView() string {
	lines := []string{
		titleStyle.Render("env-tui keybindings"),
		"",
		"  j/k, ↑/↓   move in focused panel",
		"  tab, ←/→   cycle focus: files → keys → missing",
		"  s          toggle secret obfuscation",
		"  space      reveal the focused key's values",
		"  mouse      hover a key to reveal it (select on hover)",
		"  x          toggle include source file",
		"  a          autofill selected missing key into primary .env",
		"  c          copy selected key's value to clipboard",
		"  v          toggle Code Audit (ghost / zombie / used-but-missing)",
		"  f          toggle Format & Naming Lint (bad names / syntax / whitespace)",
		"  r          rescan directory + re-audit source code",
		"  g / G      jump to top / bottom",
		"  ?          toggle this help",
		"  q          quit",
		"",
		dimStyle.Render("Values are masked by default; press s to reveal all, space to reveal the"),
		dimStyle.Render("focused key, or hover a key with the mouse."),
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

func maskVal() string {
	return "••••••••"
}

func shortName(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}
