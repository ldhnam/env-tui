// Package lint checks .env files for format and naming issues: keys that are
// not UPPER_SNAKE_CASE, malformed KEY=VALUE syntax, accidental whitespace,
// duplicate keys, and empty values.
package lint

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Kind string

const (
	BadName    Kind = "bad-name"
	Whitespace Kind = "whitespace"
	Syntax     Kind = "syntax"
	Duplicate  Kind = "duplicate"
	EmptyValue Kind = "empty-value"
)

type Issue struct {
	Path   string
	Line   int
	Key    string
	Kind   Kind
	Detail string
}

type Report struct {
	Issues []Issue            // all issues in discovery order
	ByPath map[string][]Issue // issues grouped by file path
	Files  int                // .env files scanned
}

// Scan discovers .env files in dir and lints each one.
func Scan(dir string) (*Report, error) {
	rep := &Report{ByPath: make(map[string][]Issue)}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), ".env") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		issues := lintFile(path)
		if len(issues) > 0 {
			rep.ByPath[path] = issues
			rep.Issues = append(rep.Issues, issues...)
		}
		rep.Files++
	}
	return rep, nil
}

func lintFile(path string) []Issue {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var issues []Issue
	seen := make(map[string]int) // key -> first line
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		issues = append(issues, lintLine(path, lineNo, strings.TrimRight(scanner.Text(), "\r"), seen)...)
	}
	return issues
}

var upperSnake = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func lintLine(path string, lineNo int, line string, seen map[string]int) []Issue {
	var issues []Issue
	if line == "" {
		return nil
	}
	leading := len(line) - len(strings.TrimLeft(line, " \t"))
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
		if leading > 0 {
			issues = append(issues, Issue{Path: path, Line: lineNo, Kind: Whitespace, Detail: "leading whitespace before comment"})
		}
		return issues
	}

	body := trimmed
	if strings.HasPrefix(body, "export ") {
		body = strings.TrimPrefix(body, "export ")
	}

	eq := strings.Index(body, "=")
	if eq < 0 {
		issues = append(issues, Issue{Path: path, Line: lineNo, Key: firstToken(body), Kind: Syntax, Detail: "missing '=' (expected KEY=VALUE)"})
		return issues
	}

	rawKey := body[:eq]
	rawVal := body[eq+1:]
	key := strings.TrimSpace(rawKey)
	if key == "" {
		issues = append(issues, Issue{Path: path, Line: lineNo, Kind: Syntax, Detail: "empty key name before '=' "})
		return issues
	}
	if leading > 0 {
		issues = append(issues, Issue{Path: path, Line: lineNo, Key: key, Kind: Whitespace, Detail: "leading whitespace before key"})
	}
	if rawKey != key {
		issues = append(issues, Issue{Path: path, Line: lineNo, Key: key, Kind: Whitespace, Detail: "whitespace around '='"})
	}
	if !upperSnake.MatchString(key) {
		issues = append(issues, Issue{Path: path, Line: lineNo, Key: key, Kind: BadName, Detail: "key is not UPPER_SNAKE_CASE"})
	}

	val := strings.TrimSpace(rawVal)
	if rawVal != val {
		issues = append(issues, Issue{Path: path, Line: lineNo, Key: key, Kind: Whitespace, Detail: "leading/trailing whitespace in value"})
	}
	if len(val) > 0 && (val[0] == '"' || val[0] == '\'') && (len(val) < 2 || val[len(val)-1] != val[0]) {
		issues = append(issues, Issue{Path: path, Line: lineNo, Key: key, Kind: Syntax, Detail: "unclosed quote"})
	}
	if val == "" {
		issues = append(issues, Issue{Path: path, Line: lineNo, Key: key, Kind: EmptyValue, Detail: "empty value"})
	}

	if first, ok := seen[key]; ok {
		issues = append(issues, Issue{Path: path, Line: lineNo, Key: key, Kind: Duplicate, Detail: fmt.Sprintf("duplicate key (first on line %d)", first)})
	} else {
		seen[key] = lineNo
	}
	return issues
}

func firstToken(s string) string {
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}
