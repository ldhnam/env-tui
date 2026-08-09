package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseJSONSecretMap(t *testing.T) {
	// flat form
	m, err := parseJSONSecretMap([]byte(`{"PORT":"3000","DB":"postgres://x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["PORT"] != "3000" || m["DB"] != "postgres://x" {
		t.Errorf("flat parse = %v", m)
	}
	// Doppler nested form
	m, err = parseJSONSecretMap([]byte(`{"PORT":{"raw":"3000"},"EMPTY":{"raw":""}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["PORT"] != "3000" || m["EMPTY"] != "" {
		t.Errorf("nested parse = %v", m)
	}
}

func TestParseVaultData(t *testing.T) {
	// KV v1
	m, err := parseVaultData([]byte(`{"data":{"PORT":"3000","HOST":"x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["PORT"] != "3000" || m["HOST"] != "x" {
		t.Errorf("v1 = %v", m)
	}
	// KV v2
	m, err = parseVaultData([]byte(`{"data":{"data":{"PORT":"3000"},"metadata":{"version":2}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["PORT"] != "3000" {
		t.Errorf("v2 = %v", m)
	}
}

func TestParseKeyValueLines(t *testing.T) {
	m, err := parseKeyValueLines([]byte("# comment\nPORT=3000\nDB=postgres://x\n\nNODE=dev\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m["PORT"] != "3000" || m["DB"] != "postgres://x" || m["NODE"] != "dev" {
		t.Errorf("parse = %v", m)
	}
}

// fakeBin creates an executable named name in a temp dir and returns its dir.
func fakeBin(t *testing.T, name, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFetchDoppler(t *testing.T) {
	bin := fakeBin(t, "doppler", `echo '{"PORT":{"raw":"3000"},"DB":{"raw":"postgres://x"}}'`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	m, err := Fetch(Doppler, "proj", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if m["PORT"] != "3000" || m["DB"] != "postgres://x" {
		t.Errorf("doppler fetch = %v", m)
	}
}

func TestFetchInfisical(t *testing.T) {
	bin := fakeBin(t, "infisical", `printf 'PORT=3000\nDB=postgres://x\n'`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	m, err := Fetch(Infisical, "", "prod")
	if err != nil {
		t.Fatal(err)
	}
	if m["PORT"] != "3000" || m["DB"] != "postgres://x" {
		t.Errorf("infisical fetch = %v", m)
	}
}

func TestFetchUnknownProvider(t *testing.T) {
	if _, err := Fetch("nope", "", ""); err == nil {
		t.Error("unknown provider should error")
	}
}

func TestSupported(t *testing.T) {
	for _, p := range []Provider{Doppler, HashiVault, OnePassword, AWS, Infisical} {
		if !Supported(p) {
			t.Errorf("%s should be supported", p)
		}
	}
	if Supported("nope") {
		t.Error("nope should not be supported")
	}
}
