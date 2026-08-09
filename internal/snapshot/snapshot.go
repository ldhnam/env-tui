// Package snapshot creates and restores locally encrypted snapshots of .env
// files (AES-256-GCM, passphrase-derived key via scrypt), so you can
// experiment with configuration and roll back safely.
package snapshot

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/crypto/scrypt"
)

const dirName = ".envigator"

var magic = []byte("ENVIGATOR1")

// DirPath returns the per-project snapshot directory.
func DirPath(project string) string {
	return filepath.Join(project, dirName)
}

// List returns snapshot filenames, newest first.
func List(project string) ([]string, error) {
	entries, err := os.ReadDir(DirPath(project))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".enc" {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names, nil
}

// Create encrypts the given files into a new timestamped snapshot.
func Create(project, passphrase string, files map[string]string) (string, error) {
	payload, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	enc, err := encrypt(passphrase, payload)
	if err != nil {
		return "", err
	}
	name := time.Now().Format("20060102_150405") + ".env.enc"
	if err := os.MkdirAll(DirPath(project), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(DirPath(project), name), enc, 0o600); err != nil {
		return "", err
	}
	return name, nil
}

// Read decrypts a snapshot and returns its file map.
func Read(project, name, passphrase string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(DirPath(project), name))
	if err != nil {
		return nil, err
	}
	plain, err := decrypt(passphrase, data)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s: %w (wrong passphrase?)", name, err)
	}
	var files map[string]string
	if err := json.Unmarshal(plain, &files); err != nil {
		return nil, err
	}
	return files, nil
}

// Delete removes a snapshot.
func Delete(project, name string) error {
	return os.Remove(filepath.Join(DirPath(project), name))
}

// --- crypto ---

// encrypt seals data with AES-256-GCM. Layout:
//
//	magic | salt(16) | nonce(12) | ciphertext
func encrypt(passphrase string, data []byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := scrypt.Key([]byte(passphrase), salt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{}, magic...)
	out = append(out, salt...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, data, nil), nil
}

func decrypt(passphrase string, data []byte) ([]byte, error) {
	if len(data) < len(magic)+16+12 {
		return nil, fmt.Errorf("snapshot too short")
	}
	if string(data[:len(magic)]) != string(magic) {
		return nil, fmt.Errorf("not an envigator snapshot")
	}
	rest := data[len(magic):]
	salt := rest[:16]
	nonce := rest[16 : 16+12]
	ct := rest[16+12:]
	key, err := scrypt.Key([]byte(passphrase), salt, 1<<15, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	return plain, nil
}
