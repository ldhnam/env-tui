package envfile

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func keyNameValid(key string) bool {
	return keyRe.MatchString(key)
}

// File is a parsed environment source. It may represent a local .env file
// or a remote secret store (marked with Remote).
type File struct {
	Path   string
	Name   string
	Remote bool
	Keys   []string // insertion order
	Values map[string]string
}

func (f *File) Add(key, value string) {
	if _, ok := f.Values[key]; !ok {
		f.Keys = append(f.Keys, key)
	}
	f.Values[key] = value
}

func (f *File) Has(key string) bool {
	_, ok := f.Values[key]
	return ok
}

func (f *File) Label() string {
	if f.Remote {
		return f.Name + " (remote)"
	}
	return f.Name
}

// Parse reads and parses a dotenv-style file at path.
func Parse(path string, remote bool) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f := &File{
		Path:   path,
		Name:   filepath.Base(path),
		Remote: remote,
		Values: make(map[string]string),
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if key, value, ok := splitLine(line); ok {
			f.Add(key, value)
		}
	}
	return f, scanner.Err()
}

func splitLine(line string) (string, string, bool) {
	line = strings.TrimPrefix(strings.TrimSpace(line), "export ")
	line = strings.TrimSpace(line)
	idx := strings.Index(line, "=")
	if idx <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	if !keyNameValid(key) {
		return "", "", false
	}
	raw := strings.TrimSpace(line[idx+1:])
	return key, unquote(raw), true
}

func unquote(s string) string {
	if len(s) >= 2 {
		open, close := s[0], s[len(s)-1]
		if (open == '"' && close == '"') || (open == '\'' && close == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// Discover returns sorted paths of dotenv files in dir.
func Discover(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".env") {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// Primary picks the local file used as the reference for "missing" checks.
// Preference: .env.local, then .env, then the first discovered file.
func Primary(paths []string) string {
	for _, p := range paths {
		if filepath.Base(p) == ".env.local" {
			return p
		}
	}
	for _, p := range paths {
		if filepath.Base(p) == ".env" {
			return p
		}
	}
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

// IsRemote classifies a file as a deployed/remote environment source.
// Local-ish names (.env, .env.local, .env.example, .env.development) are
// treated as local; everything else (.env.staging, .env.production, ...)
// is treated as remote.
func IsRemote(path string) bool {
	b := strings.TrimPrefix(filepath.Base(path), ".env")
	b = strings.TrimLeft(b, ".")
	switch strings.ToLower(b) {
	case "", "local", "example", "development", "dev", "local.example":
		return false
	default:
		return true
	}
}
