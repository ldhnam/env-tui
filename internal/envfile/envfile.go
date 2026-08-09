package envfile

import (
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

// File is a parsed environment source. It may represent a local .env file,
// a remote secret store (marked with Remote), or an in-memory source like a
// secret-manager vault (marked with Virtual).
type File struct {
	Path    string
	Name    string
	Remote  bool
	Virtual bool     // in-memory source (e.g. secret manager), not on disk
	Keys    []string // insertion order
	Values  map[string]string
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
	if f.Virtual {
		return f.Name
	}
	if f.Remote {
		return f.Name + " (remote)"
	}
	return f.Name
}

// Parse reads and parses a dotenv-style file at path. Values wrapped in
// quotes may span multiple physical lines (e.g. PEM keys, JSON payloads).
func Parse(path string, remote bool) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseContent(string(data), path, remote)
}

// ParseContent parses dotenv content without touching the filesystem.
func ParseContent(content, path string, remote bool) (*File, error) {
	f := &File{
		Path:   path,
		Name:   filepath.Base(path),
		Remote: remote,
		Values: make(map[string]string),
	}
	for _, line := range logicalLines(content) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if key, value, ok := splitLine(line); ok {
			f.Add(key, value)
		}
	}
	return f, nil
}

// logicalLines splits content into assignments, joining lines that fall
// inside an unclosed quoted value (multi-line PEM keys, JSON payloads).
func logicalLines(content string) []string {
	lines := strings.Split(content, "\n")
	var out []string
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			out = append(out, line)
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq > 0 && HasUnclosedQuote(trimmed[eq+1:]) {
			val := trimmed[eq+1:]
			j := i + 1
			for j < len(lines) && HasUnclosedQuote(val) {
				val += "\n" + lines[j]
				j++
			}
			out = append(out, line+"\n"+strings.Join(lines[i+1:j], "\n"))
			i = j - 1
			continue
		}
		out = append(out, line)
	}
	return out
}

// unclosedQuote reports whether s starts a quoted value that is not closed
// on this line (accounting for backslash escapes).
func HasUnclosedQuote(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 0 || (s[0] != '"' && s[0] != '\'') {
		return false
	}
	q := s[0]
	escaped := false
	for i := 1; i < len(s); i++ {
		switch {
		case escaped:
			escaped = false
		case s[i] == '\\':
			escaped = true
		case s[i] == q:
			return false
		}
	}
	return true
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
	return key, Unquote(raw), true
}

func Unquote(s string) string {
	if len(s) >= 2 {
		open, close := s[0], s[len(s)-1]
		if open == '"' && close == '"' {
			return unescapeDouble(s[1 : len(s)-1])
		}
		if open == '\'' && close == '\'' {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// unescapeDouble expands backslash escapes inside a double-quoted value.
func unescapeDouble(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteByte('\\')
				b.WriteByte(s[i])
			}
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// QuoteValue renders a value for writing back to a .env file. Values with
// newlines, quotes, or backslashes are written as a single-line double-quoted
// string with backslash escapes.
func QuoteValue(v string) string {
	if !strings.ContainsAny(v, "\n\r\"\\") {
		return v
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
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

// LoadForRun returns the environment loaded from dir's primary .env file,
// with gaps filled from the other discovered files. It never writes to disk.
func LoadForRun(dir string) (map[string]string, error) {
	paths, err := Discover(dir)
	if err != nil {
		return nil, err
	}
	primary := Primary(paths)
	envs := make(map[string]*File, len(paths))
	var files []*File
	for _, p := range paths {
		f, perr := Parse(p, IsRemote(p))
		if perr != nil {
			f = &File{Path: p, Name: filepath.Base(p), Values: make(map[string]string)}
		}
		envs[p] = f
		files = append(files, f)
	}
	env := make(map[string]string)
	if pf := envs[primary]; pf != nil {
		for _, k := range pf.Keys {
			env[k] = pf.Values[k]
		}
	}
	for _, f := range files {
		if f.Path == primary {
			continue
		}
		for _, k := range f.Keys {
			if _, ok := env[k]; !ok {
				env[k] = f.Values[k]
			}
		}
	}
	return env, nil
}

// PrimaryPath returns the primary .env path for dir, or "".
func PrimaryPath(dir string) (string, error) {
	paths, err := Discover(dir)
	if err != nil {
		return "", err
	}
	return Primary(paths), nil
}
