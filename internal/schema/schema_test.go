package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSchema(t *testing.T, dir, yaml string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".envigator.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidate(t *testing.T) {
	writeSchema(t, ".", "")
	s := &Schema{Variables: map[string]VarDef{
		"PORT":      {Required: true, Type: "integer", Default: "8080"},
		"LOG_LEVEL": {Required: true, Enum: []string{"debug", "info", "warn", "error"}},
		"REDIS_URL": {Required: true, Secret: true},
	}}
	env := map[string]string{
		"PORT":      "not-a-number",
		"LOG_LEVEL": "verbose",
		"REDIS_URL": "redis://x",
	}
	results := s.Validate(env)
	byName := map[string]Result{}
	for _, r := range results {
		byName[r.Name] = r
	}
	if byName["PORT"].OK {
		t.Error("PORT=not-a-number should fail integer type")
	}
	if byName["LOG_LEVEL"].OK {
		t.Error("LOG_LEVEL=verbose should fail enum")
	}
	if !byName["REDIS_URL"].OK {
		t.Error("REDIS_URL present should pass")
	}
	if !byName["REDIS_URL"].Secret {
		t.Error("REDIS_URL should be marked secret")
	}
}

func TestValidateRequiredMissing(t *testing.T) {
	s := &Schema{Variables: map[string]VarDef{
		"DATABASE_URL": {Required: true},
		"PORT":         {Required: false, Default: "8080"},
	}}
	results := s.Validate(map[string]string{})
	byName := map[string]Result{}
	for _, r := range results {
		byName[r.Name] = r
	}
	if byName["DATABASE_URL"].OK {
		t.Error("required missing var should fail")
	}
	if !byName["PORT"].OK {
		t.Error("optional missing var with default should pass")
	}
}

func TestRender(t *testing.T) {
	out := Render("staging", []Result{
		{Name: "DATABASE_URL", OK: true},
		{Name: "LOG_LEVEL", OK: false, Detail: "expected: one of [debug, info]\n  actual:   verbose"},
	})
	if !strings.Contains(out, "Environment: staging") {
		t.Errorf("render missing header:\n%s", out)
	}
	if !strings.Contains(out, "✓ DATABASE_URL") || !strings.Contains(out, "✗ LOG_LEVEL") {
		t.Errorf("render missing markers:\n%s", out)
	}
	if !strings.Contains(out, "1 error") {
		t.Errorf("render missing error count:\n%s", out)
	}
}

func TestLoadAndPickEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeSchema(t, dir, `
environments:
  local:
    file: .env.local
  staging:
    file: .env.staging
variables:
  PORT:
    required: true
    type: integer
`)
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.PickEnvironment("", ""); got != "local" {
		t.Errorf("pick = %s, want local", got)
	}
	if got := s.PickEnvironment("staging", ""); got != "staging" {
		t.Errorf("pick staging = %s", got)
	}
	if f := s.File(dir, "staging"); f != filepath.Join(dir, ".env.staging") {
		t.Errorf("file = %s", f)
	}
}
