package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	content := "# comment\nNODE_ENV=production\nPORT=3000\nexport SECRET=\"quoted val\"\nEMPTY=\nBAD LINE\n   \n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Parse(p, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Values["NODE_ENV"]; got != "production" {
		t.Errorf("NODE_ENV = %q", got)
	}
	if got := f.Values["PORT"]; got != "3000" {
		t.Errorf("PORT = %q", got)
	}
	if got := f.Values["SECRET"]; got != "quoted val" {
		t.Errorf("SECRET = %q", got)
	}
	if got := f.Values["EMPTY"]; got != "" {
		t.Errorf("EMPTY = %q", got)
	}
	if _, ok := f.Values["BAD"]; ok {
		t.Error("BAD LINE should not parse")
	}
	if len(f.Keys) != 4 {
		t.Errorf("keys = %v", f.Keys)
	}
}

func TestDiscoverAndPrimary(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{".env", ".env.local", ".env.example", ".env.staging", "other.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("A=1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 4 {
		t.Fatalf("discovered %d, want 4", len(paths))
	}
	if got := Primary(paths); filepath.Base(got) != ".env.local" {
		t.Errorf("primary = %s, want .env.local", got)
	}
}

func TestIsRemote(t *testing.T) {
	for name, want := range map[string]bool{
		".env":             false,
		".env.local":       false,
		".env.example":     false,
		".env.development": false,
		".env.staging":     true,
		".env.production":  true,
		".env.stage.local": true,
	} {
		if got := IsRemote(name); got != want {
			t.Errorf("IsRemote(%s) = %v, want %v", name, got, want)
		}
	}
}
