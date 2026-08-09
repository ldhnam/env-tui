package tui

import (
	"sort"
	"strings"
)

// fuzzyScore scores how well query matches s as a fuzzy subsequence.
// Returns -1 when there is no match; higher is better. Matching rewards
// consecutive characters, word-boundary starts, and early position.
func fuzzyScore(query, s string) int {
	query = strings.ToLower(query)
	s = strings.ToLower(s)
	if query == "" {
		return 1
	}
	score := 0
	prev := -2
	qi := 0
	for i := 0; i < len(s) && qi < len(query); i++ {
		if s[i] != query[qi] {
			continue
		}
		switch {
		case i == prev+1:
			score += 3
		case i == 0 || s[i-1] == '_' || s[i-1] == '.' || s[i-1] == '-' || s[i-1] == ' ':
			score += 2
		default:
			score += 1
		}
		score -= i / 10
		prev = i
		qi++
	}
	if qi != len(query) {
		return -1
	}
	return score
}

// computeSearch ranks display keys whose name or value fuzzy-matches query.
// Returns indices into displayKeys(), best matches first.
func (m Model) computeSearch(query string) []int {
	items := m.displayKeys()
	type scored struct {
		idx   int
		score int
	}
	var matches []scored
	for i, it := range items {
		val, _ := m.valueFor(it.key)
		s := max(fuzzyScore(query, it.key), fuzzyScore(query, val))
		if s < 0 {
			continue
		}
		matches = append(matches, scored{idx: i, score: s})
	}
	sort.Slice(matches, func(a, b int) bool {
		if matches[a].score != matches[b].score {
			return matches[a].score > matches[b].score
		}
		return matches[a].idx < matches[b].idx
	})
	out := make([]int, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.idx)
	}
	return out
}
