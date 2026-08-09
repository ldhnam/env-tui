package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanMultiLanguage(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "app.js", `
const port = process.env.PORT || 3000;
const db = process.env["DATABASE_URL"];
const api = process.env.API_KEY;
const mode = import.meta.env.MODE;
`)
	write(t, dir, "main.go", `
dsn := os.Getenv("DATABASE_URL")
if v := os.LookupEnv("REDIS_URL"); v != "" { }
`)
	write(t, dir, "app.py", `
import os
db = os.environ["DATABASE_URL"]
api = os.environ.get("API_KEY", "")
port = os.getenv("PORT")
`)
	write(t, dir, "lib.rs", `
let key = env::var("STRIPE_SECRET_KEY")?;
let ver = env!("CARGO_PKG_VERSION");
`)
	write(t, dir, "config.php", `
$db = getenv("DATABASE_URL");
$api = $_ENV["API_KEY"];
$host = $_SERVER["HOST"];
`)
	write(t, dir, "deploy.sh", `
echo "db=$DATABASE_URL"
echo "port=${PORT:-3000}"
`)
	write(t, dir, "docker-compose.yml", `
services:
  app:
    environment:
      - REDIS_URL=${REDIS_URL}
`)

	rep, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]int{
		"PORT":              3, // js + py + sh ($PORT / ${PORT:-3000})
		"DATABASE_URL":      5, // js + go + py + php + sh
		"API_KEY":           3, // js + py + php
		"MODE":              1,
		"REDIS_URL":         2, // go lookup + compose
		"STRIPE_SECRET_KEY": 1,
		"CARGO_PKG_VERSION": 1,
		"HOST":              1,
	}
	for key, want := range cases {
		u := rep.Usages[key]
		if u == nil {
			t.Errorf("key %s not found in audit", key)
			continue
		}
		if u.Count != want {
			t.Errorf("key %s count = %d, want %d (files %v)", key, u.Count, want, u.Files)
		}
	}
	if rep.Files == 0 {
		t.Error("no files scanned")
	}
}

func TestScanSkipsHiddenAndVendor(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "src/app.js", "process.env.PORT")
	write(t, dir, "node_modules/pkg/index.js", "process.env.NODE_ENV")
	write(t, dir, ".git/secret.js", "process.env.GIT_VAR")
	write(t, dir, "vendor/lib.go", `os.Getenv("VENDOR_VAR")`)
	write(t, dir, "dist/bundle.js", "process.env.DIST_VAR")

	rep, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if u := rep.Usages["PORT"]; u == nil || u.Count != 1 {
		t.Errorf("PORT = %+v, want 1 ref", u)
	}
	for _, forbidden := range []string{"NODE_ENV", "GIT_VAR", "VENDOR_VAR", "DIST_VAR"} {
		if u := rep.Usages[forbidden]; u != nil {
			t.Errorf("%s should have been skipped, got %+v", forbidden, u)
		}
	}
}

func TestScanDestructure(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "cfg.ts", `
const { PORT, DATABASE_URL: db, API_KEY } = process.env;
const { MODE } = import.meta.env;
`)
	rep, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"PORT", "API_KEY", "MODE"} {
		if u := rep.Usages[key]; u == nil {
			t.Errorf("%s not detected via destructuring", key)
		}
	}
}

func TestScanIgnoresEnvFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".env", "PORT=3000\nDATABASE_URL=postgres://x")
	write(t, dir, "app.js", "process.env.PORT")
	rep, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if u := rep.Usages["DATABASE_URL"]; u != nil {
		t.Errorf(".env should not be scanned for DATABASE_URL, got %+v", u)
	}
	if u := rep.Usages["PORT"]; u == nil {
		t.Error("app.js PORT should be found")
	}
}

func TestSorted(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.js", "process.env.ZEBRA; process.env.ALPHA")
	rep, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	sorted := rep.Sorted()
	if len(sorted) != 2 || sorted[0].Key != "ALPHA" || sorted[1].Key != "ZEBRA" {
		t.Errorf("sorted = %v", sorted)
	}
}
