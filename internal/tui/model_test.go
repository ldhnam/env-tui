package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"env-tui/internal/diff"
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
	if !strings.Contains(v, "env-tui") {
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
	for i := 0; i < 6; i++ {
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
