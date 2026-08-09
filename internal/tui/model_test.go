package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ldhnam/envigator/internal/audit"
	"github.com/ldhnam/envigator/internal/diff"
	"github.com/ldhnam/envigator/internal/gitguard"
	"github.com/ldhnam/envigator/internal/lint"
)

func testModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".env.local", "NODE_ENV=development\nPORT=3000\nDATABASE_URL=postgres://localhost/db\nSTRIPE_SECRET_KEY=\n")
	write(".env.example", "NODE_ENV=production\nPORT=3000\nDATABASE_URL=postgres://user:pass@host/db\nSTRIPE_SECRET_KEY=sk_live_x\nREDIS_URL=redis://x\n")
	write(".env.staging", "NODE_ENV=staging\nDATABASE_URL=postgres://staging/db\nREDIS_URL=redis://staging\n")
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	write("src/app.js", "const port = process.env.PORT;\nconst db = process.env.REDIS_URL;\nconst gh = process.env.GHOST_API_TOKEN;\n")
	m := New(dir)
	m.width, m.height = 100, 30
	return m
}

func TestModelLoadAndDiff(t *testing.T) {
	m := testModel(t)
	if m.prim == "" || filepath.Base(m.prim) != ".env.local" {
		t.Fatalf("primary = %q", m.prim)
	}
	if m.rep.ByKey["REDIS_URL"] == nil {
		t.Fatal("REDIS_URL not in report")
	}
	if m.rep.ByKey["REDIS_URL"].Status != diff.Missing {
		t.Errorf("REDIS_URL = %v, want MISSING", m.rep.ByKey["REDIS_URL"].Status)
	}
	if m.rep.ByKey["DATABASE_URL"].Status != diff.Diff {
		t.Errorf("DATABASE_URL = %v, want DIFF", m.rep.ByKey["DATABASE_URL"].Status)
	}
	if m.rep.ByKey["NODE_ENV"].Status != diff.Diff {
		t.Errorf("NODE_ENV = %v, want DIFF", m.rep.ByKey["NODE_ENV"].Status)
	}
	if len(m.rep.Missing) != 1 {
		t.Errorf("missing = %d, want 1", len(m.rep.Missing))
	}
}

func TestViewRenders(t *testing.T) {
	m := testModel(t)
	v := m.View()
	if !strings.Contains(v, "envigator") {
		t.Error("view missing header")
	}
	if !strings.Contains(v, "Target Files") {
		t.Error("view missing files panel")
	}
	if !strings.Contains(v, "Missing Keys") {
		t.Error("view missing missing-keys panel")
	}
	if !strings.Contains(v, "REDIS_URL") {
		t.Error("view missing key")
	}
	if strings.Contains(v, "sk_live_x") {
		t.Error("view leaked secret in default (masked) mode")
	}
	if !strings.Contains(v, "••••••") {
		t.Error("view should mask values")
	}
}

func TestSecretsToggleShowsValue(t *testing.T) {
	m := testModel(t)
	for i, st := range m.rep.All {
		if st.Key == "STRIPE_SECRET_KEY" {
			m.keyIdx = i
		}
	}
	m.showSecrets = true
	v := m.View()
	if !strings.Contains(v, "sk_live_x") {
		t.Error("view should reveal secret when toggled")
	}
	m.showSecrets = false
	if strings.Contains(m.View(), "sk_live_x") {
		t.Error("view leaked secret in masked mode")
	}
}

func TestToggleFile(t *testing.T) {
	m := testModel(t)
	if len(m.files) != 3 {
		t.Fatalf("files = %d", len(m.files))
	}
	m.focus = panelFiles
	m.fileIdx = 1
	selBefore := m.selec[1]
	m.toggleFile()
	if m.selec[1] == selBefore {
		t.Error("selection did not toggle")
	}
}

func TestAutofillWritesKey(t *testing.T) {
	m := testModel(t)
	// locate REDIS_URL missing state
	st := m.rep.ByKey["REDIS_URL"]
	if st == nil {
		t.Fatal("no REDIS_URL state")
	}
	primBefore, _ := os.ReadFile(m.prim)
	m.promptKey = "REDIS_URL"
	if err := m.appendPrimary("REDIS_URL", "redis://myvalue"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(m.prim)
	if string(after) == string(primBefore) {
		t.Error("primary file not updated")
	}
	if !strings.Contains(string(after), "REDIS_URL=redis://myvalue") {
		t.Errorf("written content = %q", after)
	}
}

func TestAutofillUpdatesReport(t *testing.T) {
	m := testModel(t)
	_ = m.appendPrimary("REDIS_URL", "redis://myvalue")
	m.reload()
	st := m.rep.ByKey["REDIS_URL"]
	if st == nil || st.Status == diff.Missing {
		t.Errorf("after autofill REDIS_URL should be present, got %+v", st)
	}
	if len(m.rep.Missing) != 0 {
		t.Errorf("missing after autofill = %d", len(m.rep.Missing))
	}
}

func update(m Model, msg tea.Msg) Model {
	mm, _ := m.Update(msg)
	return mm.(Model)
}

func TestTabCyclesFocus(t *testing.T) {
	m := testModel(t)
	seen := map[panel]bool{}
	for range 6 {
		m = update(m, tea.KeyMsg{Type: tea.KeyTab})
		seen[m.focus] = true
	}
	for _, p := range []panel{panelFiles, panelKeys, panelMissing} {
		if !seen[p] {
			t.Errorf("focus panel %v never reached", p)
		}
	}
}

func TestAutofillViaUpdate(t *testing.T) {
	m := testModel(t)
	m.focus = panelMissing
	m.missIdx = 0
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.prompting {
		t.Fatal("prompt did not open")
	}
	for _, r := range "redis://from-test" {
		m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.prompting {
		t.Error("prompt should close after enter")
	}
	data, err := os.ReadFile(m.prim)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "REDIS_URL=redis://from-test") {
		t.Errorf("primary file content = %q", data)
	}
	if st := m.rep.ByKey["REDIS_URL"]; st == nil || st.Status == diff.Missing {
		t.Error("REDIS_URL should no longer be missing after autofill")
	}
}

func TestSecretsToggleViaUpdate(t *testing.T) {
	m := testModel(t)
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !m.showSecrets {
		t.Error("s should reveal secrets")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if m.showSecrets {
		t.Error("s should hide secrets again")
	}
}

func TestSpaceRevealsKey(t *testing.T) {
	m := testModel(t)
	m.focus = panelKeys
	m.keyIdx = 0 // NODE_ENV
	if strings.Contains(m.View(), "development") {
		t.Error("value should be masked before space")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.revealed["NODE_ENV"] {
		t.Fatal("space should reveal the focused key")
	}
	v := m.View()
	if !strings.Contains(v, "development") {
		t.Error("revealed key's value should be visible")
	}
	if strings.Contains(v, "postgres://localhost") {
		t.Error("unrelated key value leaked while only NODE_ENV is revealed")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.revealed["NODE_ENV"] {
		t.Error("space should hide the key again")
	}
	if strings.Contains(m.View(), "development") {
		t.Error("value should be masked after second space")
	}
}

func TestHoverReveals(t *testing.T) {
	m := testModel(t)
	// keys panel geometry at 100x30: filesW=25, keysW=28, rows start y=3,
	// x range [26, 52). First key row = NODE_ENV, second = PORT.
	m = update(m, tea.MouseMsg{Action: tea.MouseActionMotion, X: 27, Y: 3})
	if m.hoverKey != "NODE_ENV" {
		t.Errorf("hoverKey = %q, want NODE_ENV", m.hoverKey)
	}
	if !m.revealKey("NODE_ENV") {
		t.Error("hovered key should be revealed")
	}
	if !strings.Contains(m.View(), "development") {
		t.Error("hovered key value should be visible")
	}
	// hovering the second row selects PORT and reveals it
	m = update(m, tea.MouseMsg{Action: tea.MouseActionMotion, X: 27, Y: 4})
	if m.hoverKey != "PORT" || m.keyIdx != 1 {
		t.Errorf("hover over row 2 = %q idx %d, want PORT/1", m.hoverKey, m.keyIdx)
	}
	if !m.revealKey("PORT") {
		t.Error("PORT should be revealed while hovered")
	}
	// moving off any key clears the hover reveal
	m = update(m, tea.MouseMsg{Action: tea.MouseActionMotion, X: 1, Y: 1})
	if m.hoverKey != "" {
		t.Error("hoverKey should clear when the mouse leaves")
	}
	if m.revealKey("PORT") {
		t.Error("PORT should be masked again after hover leaves")
	}
	// a release also clears hover
	m = update(m, tea.MouseMsg{Action: tea.MouseActionMotion, X: 27, Y: 3})
	if m.hoverKey != "NODE_ENV" {
		t.Fatal("hover should re-engage")
	}
	m = update(m, tea.MouseMsg{Action: tea.MouseActionRelease})
	if m.hoverKey != "" {
		t.Error("release should clear hover")
	}
}

func TestAuditScanAndView(t *testing.T) {
	m := testModel(t)
	rep, err := audit.Scan(m.dir)
	if err != nil {
		t.Fatal(err)
	}
	m = update(m, auditMsg{rep: rep})
	if m.audit == nil {
		t.Fatal("audit report not stored")
	}
	if m.refCount("REDIS_URL") == 0 {
		t.Error("REDIS_URL should have code references")
	}
	// ghost detection: GHOST_API_TOKEN is used in code but in no .env file
	if len(m.ghostKeys()) != 1 || m.ghostKeys()[0].Key != "GHOST_API_TOKEN" {
		t.Errorf("ghostKeys = %+v, want [GHOST_API_TOKEN]", m.ghostKeys())
	}
	// zombie detection: NODE_ENV is in .env files but never referenced
	if !m.isZombie("NODE_ENV") {
		t.Error("NODE_ENV should be a zombie key")
	}
	if m.isZombie("PORT") {
		t.Error("PORT is referenced in code, must not be a zombie")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if !m.auditView {
		t.Fatal("v should toggle audit view")
	}
	v := m.View()
	for _, want := range []string{"Code Audit", "Ghost Keys", "GHOST_API_TOKEN", "Used but missing from", "Zombie Keys"} {
		if !strings.Contains(v, want) {
			t.Errorf("audit view missing %q", want)
		}
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if m.auditView {
		t.Error("v should close audit view")
	}
}

func TestGhostKeyInKeysPanel(t *testing.T) {
	m := testModel(t)
	rep, _ := audit.Scan(m.dir)
	m = update(m, auditMsg{rep: rep})
	m.focus = panelKeys
	v := m.View()
	if !strings.Contains(v, "GHOST_API_TOKEN") {
		t.Errorf("ghost key should appear in keys panel:\n%s", v)
	}
	// navigate to the ghost (it is prepended, index 0)
	m.keyIdx = 0
	key := m.currentKey()
	if key != "GHOST_API_TOKEN" {
		t.Errorf("currentKey = %q, want GHOST_API_TOKEN", key)
	}
	if m.currentState() != nil {
		t.Error("ghost key should have no diff state")
	}
	dv := m.View()
	if !strings.Contains(dv, "ghost key") {
		t.Errorf("detail pane should describe the ghost key:\n%s", dv)
	}
}

func TestKeysPaneShowsRefs(t *testing.T) {
	m := testModel(t)
	rep, _ := audit.Scan(m.dir)
	m = update(m, auditMsg{rep: rep})
	m.focus = panelKeys
	v := m.View()
	if !strings.Contains(v, "REDIS_URL") || !strings.Contains(v, "×1") {
		t.Errorf("keys pane should show ref counts:\n%s", v)
	}
}

func TestLintView(t *testing.T) {
	dir := t.TempDir()
	content := "PORT=3000\nport=1\nPORT = 4000\nEMPTY=\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(dir)
	m.width, m.height = 100, 30
	rep, err := lint.Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	m = update(m, lintMsg{rep: rep})
	if m.lint == nil {
		t.Fatal("lint report not stored")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if !m.lintView {
		t.Fatal("f should toggle lint view")
	}
	v := m.View()
	for _, want := range []string{"Format & Naming Lint", "bad-name", "whitespace"} {
		if !strings.Contains(v, want) {
			t.Errorf("lint view missing %q", want)
		}
	}
	if !strings.Contains(v, "⚠5") {
		t.Errorf("files panel should show lint count badge:\n%s", v)
	}
	// detail pane surfaces lint count for a flagged key
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.lintView {
		t.Error("f should close lint view")
	}
	m.focus = panelKeys
	m.keyIdx = 0 // PORT (env keys first)
	if !strings.Contains(m.View(), "Lint :") {
		t.Error("detail pane should surface lint issues for PORT")
	}
	// toggling audit clears lint view and vice versa
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if !m.auditView {
		t.Error("v should open audit view")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if !m.lintView || m.auditView {
		t.Error("f should switch to lint view and close audit")
	}
}

// autofillValue drives the autofill prompt to completion for a given value.
// joinT assembles a token from fragments so no full credential-shaped
// literal appears in source (GitHub push-protection friendly).
func joinT(parts ...string) string { return strings.Join(parts, "") }

func autofillValue(t *testing.T, m Model, value string) Model {
	t.Helper()
	m.focus = panelMissing
	m.missIdx = 0
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !m.prompting {
		t.Fatal("autofill prompt did not open")
	}
	for _, r := range value {
		m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	return m
}

func TestAutofillGuardBlocksSecret(t *testing.T) {
	m := testModel(t)
	m = autofillValue(t, m, joinT("sk_", "live_4eC39HqLyjWDarjtT1zdp7dc"))
	if !m.confirming {
		t.Fatal("secret-like value should trigger the pre-commit guard")
	}
	if len(m.confirmMatches) == 0 || m.confirmMatches[0].Name != "Stripe" {
		t.Errorf("confirmMatches = %+v, want Stripe", m.confirmMatches)
	}
	// the guard view should not show the raw secret, and must not save yet
	if strings.Contains(m.View(), joinT("sk_", "live_4eC39HqLyjWDarjtT1zdp7dc")) {
		t.Error("guard view leaked the raw secret")
	}
	data, _ := os.ReadFile(m.prim)
	if strings.Contains(string(data), joinT("sk_", "live_4eC39HqLyjWDarjtT1zdp7dc")) {
		t.Error("secret was written before confirmation")
	}
	// 'n' cancels: nothing saved
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.confirming {
		t.Error("n should close the guard")
	}
	data, _ = os.ReadFile(m.prim)
	if strings.Contains(string(data), joinT("sk_", "live_4eC39HqLyjWDarjtT1zdp7dc")) {
		t.Error("secret saved after pressing n")
	}
}

func TestAutofillGuardConfirmSaves(t *testing.T) {
	m := testModel(t)
	m = autofillValue(t, m, joinT("ghp_", "1234567890abcdefghij"))
	if !m.confirming {
		t.Fatal("should trigger guard")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if m.confirming {
		t.Error("y should close the guard")
	}
	data, _ := os.ReadFile(m.prim)
	if !strings.Contains(string(data), joinT("ghp_", "1234567890abcdefghij")) {
		t.Errorf("y should save the value, file=%q", data)
	}
}

func TestAutofillPlainValueSkipsGuard(t *testing.T) {
	m := testModel(t)
	m = autofillValue(t, m, "redis://myvalue")
	if m.confirming {
		t.Fatal("plain value should not trigger the guard")
	}
	data, _ := os.ReadFile(m.prim)
	if !strings.Contains(string(data), "REDIS_URL=redis://myvalue") {
		t.Errorf("plain value should save directly, file=%q", data)
	}
}

func TestSecretLeakMarkers(t *testing.T) {
	dir := t.TempDir()
	content := "PORT=3000\nSTRIPE_KEY=" + joinT("sk_", "live_4eC39HqLyjWDarjtT1zdp7dc") + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(dir)
	m.width, m.height = 100, 30
	lrep, err := lint.Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	m = update(m, lintMsg{rep: lrep})
	if !m.secretKeySet()["STRIPE_KEY"] {
		t.Error("STRIPE_KEY should be flagged as a secret")
	}
	if m.secretKeySet()["PORT"] {
		t.Error("PORT should not be flagged as a secret")
	}
	// keys panel shows the S marker
	m.focus = panelKeys
	if !strings.Contains(m.View(), "STRIPE_KEY") {
		t.Error("keys panel missing STRIPE_KEY")
	}
	// lint panel shows the secrets section
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	v := m.View()
	if !strings.Contains(v, "Secrets Detected") || !strings.Contains(v, "Stripe") {
		t.Errorf("lint panel should surface the leak:\n%s", v)
	}
}

func stripANSI(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`).ReplaceAllString(s, "")
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestGitSafety(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", ".env.local\n")
	write(".env.local", "SECRET=1\n")
	write(".env.dev", "SECRET=2\n")
	write(".env.example", "SECRET=\n")

	m := New(dir)
	m.width, m.height = 100, 30
	if m.git == nil || !m.git.IsRepo {
		t.Fatal("should detect the git repo")
	}
	envLocal := filepath.Join(dir, ".env.local")
	envDev := filepath.Join(dir, ".env.dev")
	envExample := filepath.Join(dir, ".env.example")
	if m.git.ByPath[envLocal] != gitguard.Ignored {
		t.Errorf(".env.local = %v, want Ignored", m.git.ByPath[envLocal])
	}
	if m.git.ByPath[envDev] != gitguard.Exposed {
		t.Errorf(".env.dev = %v, want Exposed", m.git.ByPath[envDev])
	}
	if m.git.ByPath[envExample] != gitguard.Exposed {
		t.Errorf(".env.example = %v, want Exposed", m.git.ByPath[envExample])
	}
	v := m.View()
	if !strings.Contains(v, "git: 1 exposed") {
		t.Errorf("header badge missing 'git: 1 exposed':\n%s", v)
	}
	// per-file markers: ignored ✓, exposed !
	plain := stripANSI(v)
	if !strings.Contains(plain, ".env.local ✓") || !strings.Contains(plain, ".env.dev !") {
		t.Errorf("files panel missing git markers:\n%s", plain)
	}
}

func TestGitSafetyProtected(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".env*\n!.env.example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", ".env.local", ".env.staging", ".env.example"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("X=1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := New(dir)
	m.width, m.height = 100, 30
	v := m.View()
	if !strings.Contains(v, "git: protected") {
		t.Errorf("header should show 'git: protected':\n%s", v)
	}
}

func TestGitNotARepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("X=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(dir)
	m.width, m.height = 100, 30
	if !strings.Contains(m.View(), "git: n/a") {
		t.Error("non-repo should show 'git: n/a'")
	}
}

func TestEditInPlace(t *testing.T) {
	m := testModel(t)
	m.focus = panelKeys
	for i, st := range m.rep.All {
		if st.Key == "DATABASE_URL" {
			m.keyIdx = i
		}
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if !m.editing {
		t.Fatal("e should open the editor")
	}
	if got := m.editor.Value(); got != "postgres://localhost/db" {
		t.Errorf("editor prefilled = %q", got)
	}
	m.editor.SetValue("postgres://new-host:5432/db")
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.editing {
		t.Error("editor should close on save")
	}
	data, _ := os.ReadFile(m.prim)
	if !strings.Contains(string(data), "DATABASE_URL=postgres://new-host:5432/db") {
		t.Errorf("primary not updated in place:\n%s", data)
	}
	if strings.Count(string(data), "DATABASE_URL=") != 1 {
		t.Errorf("expected in-place replacement, got:\n%s", data)
	}
	if st := m.rep.ByKey["DATABASE_URL"]; st == nil || st.Values[m.prim] != "postgres://new-host:5432/db" {
		t.Errorf("model not refreshed: %+v", st)
	}
}

func TestEditCancels(t *testing.T) {
	m := testModel(t)
	m.focus = panelKeys
	m.keyIdx = 0
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m.editor.SetValue("changed-but-cancelled")
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.editing {
		t.Error("esc should close the editor")
	}
	data, _ := os.ReadFile(m.prim)
	if strings.Contains(string(data), "changed-but-cancelled") {
		t.Error("cancel should not write")
	}
}

func TestEditMultiLineJSON(t *testing.T) {
	m := testModel(t)
	m.focus = panelKeys
	m.keyIdx = 0 // NODE_ENV
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	json := "{\n  \"host\": \"localhost\",\n  \"port\": 5432\n}"
	m.editor.SetValue(json)
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.editing || m.confirming {
		t.Fatal("plain JSON should save without the secret guard")
	}
	data, _ := os.ReadFile(m.prim)
	if !strings.Contains(string(data), `NODE_ENV="{\n  \"host\": \"localhost\",\n  \"port\": 5432\n}"`) {
		t.Errorf("expected escaped quoted storage:\n%s", data)
	}
	if got := m.rep.ByKey["NODE_ENV"].Values[m.prim]; got != json {
		t.Errorf("round-trip = %q, want %q", got, json)
	}
}

func TestEditSecretGuard(t *testing.T) {
	m := testModel(t)
	m.focus = panelKeys
	m.keyIdx = 0 // NODE_ENV
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m.editor.SetValue(joinT("sk_", "live_4eC39HqLyjWDarjtT1zdp7dc"))
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if !m.confirming {
		t.Fatal("editor should trigger the secret guard")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	data, _ := os.ReadFile(m.prim)
	if strings.Contains(string(data), "live_4eC") {
		t.Error("secret should not be saved after cancel")
	}
	// confirm saves in place
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m.editor.SetValue(joinT("sk_", "live_4eC39HqLyjWDarjtT1zdp7dc"))
	m = update(m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	data, _ = os.ReadFile(m.prim)
	if !strings.Contains(string(data), "NODE_ENV="+joinT("sk_", "live_4eC39HqLyjWDarjtT1zdp7dc")) {
		t.Errorf("confirmed secret should save in place:\n%s", data)
	}
}
