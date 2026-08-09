package tui

import (
	"sort"
	"strings"

	"github.com/ldhnam/env-tui/internal/secrets"
)

type secretHit struct {
	Path    string
	Key     string
	Pattern string
}

// secretKeySet returns keys whose value in any discovered .env file matches a
// known credential pattern (potential leak).
func (m Model) secretKeySet() map[string]bool {
	set := make(map[string]bool)
	for _, f := range m.files {
		for k, v := range f.Values {
			if len(secrets.Detect(v)) > 0 {
				set[k] = true
			}
		}
	}
	return set
}

// secretHits lists every (file, key, pattern) credential match across all
// discovered .env files, deduplicated and sorted by file then key.
func (m Model) secretHits() []secretHit {
	var out []secretHit
	for _, f := range m.files {
		for _, k := range f.Keys {
			for _, mt := range secrets.Detect(f.Values[k]) {
				out = append(out, secretHit{Path: f.Path, Key: k, Pattern: mt.Name})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// keySecrets returns the unique credential patterns detected across the
// selected sources for a single key.
func (m Model) keySecrets(key string) []secrets.Match {
	var out []secrets.Match
	seen := make(map[string]bool)
	for i, f := range m.files {
		if i >= len(m.selec) || !m.selec[i] {
			continue
		}
		for _, mt := range secrets.Detect(f.Values[key]) {
			if !seen[mt.Name] {
				seen[mt.Name] = true
				out = append(out, mt)
			}
		}
	}
	return out
}

func joinPatterns(matches []secrets.Match) string {
	names := make([]string, 0, len(matches))
	for _, mt := range matches {
		names = append(names, mt.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
