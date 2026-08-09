package diff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ldhnam/env-tui/internal/envfile"
)

func env(t *testing.T, path, body string) *envfile.File {
	t.Helper()
	p := filepath.Join(t.TempDir(), filepath.Base(path))
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := envfile.Parse(p, false)
	if err != nil {
		t.Fatal(err)
	}
	f.Path = path
	return f
}

func TestBuild(t *testing.T) {
	envs := map[string]*envfile.File{
		"a": env(t, "a", "COMMON=1\nLOCAL=2\nMISSING_IN_PRIM=9\n"),
		"b": env(t, "b", "COMMON=1\nOTHER=3\n"),
	}
	report := Build("a", []string{"a", "b"}, envs)

	common := report.ByKey["COMMON"]
	if common.Status != Match {
		t.Errorf("COMMON status = %v, want MATCH", common.Status)
	}
	other := report.ByKey["OTHER"]
	if other.Status != Missing {
		t.Errorf("OTHER status = %v, want MISSING", other.Status)
	}
	if len(report.Missing) != 1 || report.Missing[0].Key != "OTHER" {
		t.Errorf("missing = %+v", report.Missing)
	}
	if !other.Present["b"] || other.Present["a"] {
		t.Error("OTHER presence wrong")
	}
	src := other.Sources("a", []string{"a", "b"})
	if len(src) != 1 || src[0] != "b" {
		t.Errorf("sources = %v", src)
	}
}

func TestDiffStatus(t *testing.T) {
	envs := map[string]*envfile.File{
		"a": env(t, "a", "K=v1\n"),
		"b": env(t, "b", "K=v2\n"),
	}
	report := Build("a", []string{"a", "b"}, envs)
	if report.ByKey["K"].Status != Diff {
		t.Errorf("K status = %v, want DIFF", report.ByKey["K"].Status)
	}
}

func TestFormatValid(t *testing.T) {
	envs := map[string]*envfile.File{
		"a": env(t, "a", "GOOD=123\n"),
		"b": env(t, "b", "BAD KEY=1\nEMPTY=\n"),
	}
	report := Build("a", []string{"a", "b"}, envs)
	if !FormatValid(report.ByKey["GOOD"]) {
		t.Error("GOOD should be valid")
	}
	if FormatValid(report.ByKey["EMPTY"]) {
		t.Error("EMPTY should be invalid")
	}
}
