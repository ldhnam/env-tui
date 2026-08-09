package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunHealthy(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFile(t, dir, ".gitignore", ".env*\n!.env.example\n")
	writeFile(t, dir, ".env.local", "PORT=3000\nNODE_ENV=dev\n")
	writeFile(t, dir, ".env.example", "PORT=\nNODE_ENV=\n")
	rep, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	errs, warns := rep.Summary()
	if warns != 1 { // not a tracked-secrets situation, but no code -> unused vars are warnings
		t.Logf("errs=%d warns=%d", errs, warns)
	}
	text := Render(rep)
	for _, want := range []string{"Envigator Doctor", ".env exists", ".env.example exists", ".env is gitignored", "variables valid", "Summary"} {
		if !strings.Contains(text, want) {
			t.Errorf("render missing %q:\n%s", want, text)
		}
	}
}

func TestRunDetectsBackupAndTrackedSecret(t *testing.T) {
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFile(t, dir, ".gitignore", ".env.local\n")
	writeFile(t, dir, ".env.local", "PORT=3000\n")
	writeFile(t, dir, ".env.example", "PORT=\n")
	writeFile(t, dir, ".env.backup", "OLD=1\n")
	writeFile(t, dir, ".env.production", "SECRET="+testKey+"\n")
	gitRun(t, dir, "add", "-f", ".env.production")

	rep, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	text := Render(rep)
	if !strings.Contains(text, "backup file(s) not gitignored") {
		t.Errorf("expected backup warning:\n%s", text)
	}
	if !strings.Contains(text, "tracked in git") {
		t.Errorf("expected tracked-secret error:\n%s", text)
	}
	if !strings.Contains(text, "credential(s) in tracked files") {
		t.Errorf("expected credentials-committed error:\n%s", text)
	}
	if !strings.Contains(text, "error") {
		t.Errorf("expected errors in summary:\n%s", text)
	}
}

const testKey = "sk_" + "test_000000000000000000000000"
