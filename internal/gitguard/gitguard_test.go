package gitguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckNonRepo(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	writeFile(t, p, "PORT=3000\n")
	rep := Check(dir, []string{p})
	if rep.IsRepo {
		t.Error("temp dir should not be a git repo")
	}
	if rep.ByPath[p] != NotRepo {
		t.Errorf("status = %v, want NotRepo", rep.ByPath[p])
	}
}

func TestCheckIgnoredAndExposed(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	writeFile(t, filepath.Join(dir, ".gitignore"), ".env.local\n")
	ignored := filepath.Join(dir, ".env.local")
	exposed := filepath.Join(dir, ".env.production")
	template := filepath.Join(dir, ".env.example")
	for _, f := range []string{ignored, exposed, template} {
		writeFile(t, f, "PORT=3000\n")
	}
	rep := Check(dir, []string{ignored, exposed, template})
	if !rep.IsRepo {
		t.Fatal("should be a git repo")
	}
	if rep.ByPath[ignored] != Ignored {
		t.Errorf(".env.local = %v, want Ignored", rep.ByPath[ignored])
	}
	if rep.ByPath[exposed] != Exposed {
		t.Errorf(".env.production = %v, want Exposed", rep.ByPath[exposed])
	}
	if rep.ByPath[template] != Exposed {
		t.Errorf(".env.example (not ignored) = %v, want Exposed", rep.ByPath[template])
	}
}

func TestCheckTracked(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "t@t")
	git(t, dir, "config", "user.name", "t")
	writeFile(t, filepath.Join(dir, ".gitignore"), ".env*\n")
	env := filepath.Join(dir, ".env.local")
	writeFile(t, env, "PORT=3000\n")
	git(t, dir, "add", "-f", ".env.local")
	git(t, dir, "commit", "-q", "-m", "init")
	// even though it matches .gitignore, a tracked file stays tracked
	rep := Check(dir, []string{env})
	if rep.ByPath[env] != Tracked {
		t.Errorf("tracked .env.local = %v, want Tracked", rep.ByPath[env])
	}
}
