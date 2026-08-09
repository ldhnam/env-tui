package snapshot

import (
	"os"
	"strings"
	"testing"
)

func TestCreateReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		".env":       "PORT=3000\nSECRET=x\n",
		".env.local": "PORT=3001\n",
	}
	name, err := Create(dir, "hunter2", files)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, ".env.enc") {
		t.Errorf("name = %s", name)
	}
	// listed
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0] != name {
		t.Errorf("list = %v", list)
	}
	// on-disk content must not contain plaintext
	data, _ := os.ReadFile(DirPath(dir) + "/" + name)
	if strings.Contains(string(data), "PORT=3000") {
		t.Error("snapshot stored plaintext")
	}
	// round-trip
	got, err := Read(dir, name, "hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if got[".env"] != files[".env"] || got[".env.local"] != "PORT=3001\n" {
		t.Errorf("round-trip = %v", got)
	}
}

func TestWrongPassphraseFails(t *testing.T) {
	dir := t.TempDir()
	name, err := Create(dir, "correct", map[string]string{".env": "A=1\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dir, name, "wrong"); err == nil {
		t.Error("wrong passphrase should fail to decrypt")
	}
}

func TestDeleteAndEmptyList(t *testing.T) {
	dir := t.TempDir()
	list, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %v", list)
	}
	name, err := Create(dir, "p", map[string]string{".env": "A=1\n"})
	if err != nil {
		t.Fatal(err)
	}
	if err := Delete(dir, name); err != nil {
		t.Fatal(err)
	}
	list, _ = List(dir)
	if len(list) != 0 {
		t.Errorf("after delete list = %v", list)
	}
}

func TestDecryptInvalidData(t *testing.T) {
	if _, err := decrypt("p", []byte("not-a-snapshot")); err == nil {
		t.Error("garbage should fail to decrypt")
	}
}
