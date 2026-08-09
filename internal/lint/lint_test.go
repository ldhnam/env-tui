package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintLine(t *testing.T) {
	cases := []struct {
		line  string
		kinds []Kind
	}{
		{"PORT=3000", nil},
		{"export DATABASE_URL=postgres://x", nil},
		{"KEY='quoted'", nil},
		{"  INDENTED=1", []Kind{Whitespace}},
		{"  # comment", []Kind{Whitespace}},
		{"# comment", nil},
		{"PORT = 3000", []Kind{Whitespace, Whitespace}}, // around '=' and in value
		{"PORT= 3000", []Kind{Whitespace}},
		{"port=3000", []Kind{BadName}},
		{"MY-KEY=1", []Kind{BadName}},
		{"123KEY=1", []Kind{BadName}},
		{"=value", []Kind{Syntax}},
		{"SOME_KEY value", []Kind{Syntax}},
		{"KEY=\"unclosed", []Kind{Syntax}},
		{"EMPTY=", []Kind{EmptyValue}},
		{"", nil},
		{"   ", nil},
	}
	for _, c := range cases {
		seen := make(map[string]int)
		got := lintLine("f", 1, c.line, seen)
		if len(got) != len(c.kinds) {
			t.Errorf("lintLine(%q) = %v, want kinds %v", c.line, got, c.kinds)
			continue
		}
		for i, g := range got {
			if g.Kind != c.kinds[i] {
				t.Errorf("lintLine(%q)[%d].Kind = %v, want %v", c.line, i, g.Kind, c.kinds[i])
			}
		}
	}
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	content := strings.Join([]string{
		"PORT=3000",
		"WHITESPACE = 4000", // whitespace
		"lowercase=3000",    // bad-name
		"MY-KEY=1",          // bad-name
		"=value",            // syntax
		"SOME_KEY val",      // syntax
		"KEY=\"unclosed",    // syntax
		"  INDENTED=1",      // whitespace
		"DUP=1",
		"DUP=2",  // duplicate
		"EMPTY=", // empty-value
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Files != 1 {
		t.Errorf("files = %d, want 1", rep.Files)
	}
	counts := map[Kind]int{}
	for _, iss := range rep.Issues {
		counts[iss.Kind]++
	}
	want := map[Kind]int{
		Whitespace: 3, // "WHITESPACE = 4000" flags around-= + in-value, plus INDENTED leading
		BadName:    2,
		Syntax:     3,
		Duplicate:  1,
		EmptyValue: 1,
	}
	for k, w := range want {
		if counts[k] != w {
			t.Errorf("kind %s = %d, want %d (all: %v)", k, counts[k], w, counts)
		}
	}
	if len(rep.Issues) != 10 {
		t.Errorf("total issues = %d, want 10", len(rep.Issues))
	}
}

func TestScanEmptyDir(t *testing.T) {
	dir := t.TempDir()
	rep, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Issues) != 0 || rep.Files != 0 {
		t.Errorf("rep = %+v, want empty", rep)
	}
}

func TestScanIgnoresNonEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("port=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PORT=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, _ := Scan(dir)
	if rep.Files != 1 || len(rep.Issues) != 0 {
		t.Errorf("rep = files %d issues %d, want 1/0", rep.Files, len(rep.Issues))
	}
}
