package diff

import (
	"github.com/ldhnam/env-tui/internal/envfile"
	"regexp"
)

type Status int

const (
	Match Status = iota
	Diff
	Missing
)

func (s Status) String() string {
	switch s {
	case Match:
		return "MATCH"
	case Diff:
		return "DIFF"
	case Missing:
		return "MISSING"
	}
	return "UNKNOWN"
}

// KeyState describes one environment key across all selected sources.
type KeyState struct {
	Key     string
	Values  map[string]string // source path -> value (present sources only)
	Present map[string]bool   // source path -> present
	Status  Status
}

// Sources returns the paths (other than primary) where the key exists.
func (k *KeyState) Sources(primary string, files []string) []string {
	var out []string
	for _, f := range files {
		if f == primary {
			continue
		}
		if k.Present[f] {
			out = append(out, f)
		}
	}
	return out
}

// Report is the computed diff across a set of selected sources.
type Report struct {
	Primary string
	Files   []string    // selected source paths in display order
	All     []*KeyState // union of keys, primary-file order first
	Missing []*KeyState // keys absent from the primary source
	ByKey   map[string]*KeyState
}

func Build(primary string, files []string, envs map[string]*envfile.File) *Report {
	r := &Report{
		Primary: primary,
		Files:   files,
		ByKey:   make(map[string]*KeyState),
	}
	if primary == "" || len(files) == 0 {
		return r
	}

	seen := make(map[string]bool)
	for _, f := range files {
		file := envs[f]
		if file == nil {
			continue
		}
		for _, key := range file.Keys {
			if seen[key] {
				continue
			}
			seen[key] = true
			r.All = append(r.All, r.stateFor(key, files, envs))
		}
	}

	for _, st := range r.All {
		if st.Status == Missing {
			r.Missing = append(r.Missing, st)
		}
	}
	return r
}

func (r *Report) stateFor(key string, files []string, envs map[string]*envfile.File) *KeyState {
	st := &KeyState{
		Key:     key,
		Values:  make(map[string]string),
		Present: make(map[string]bool),
	}
	var present []string
	for _, f := range files {
		if file := envs[f]; file != nil && file.Has(key) {
			st.Present[f] = true
			st.Values[f] = file.Values[key]
			present = append(present, file.Values[key])
		}
	}
	if !st.Present[r.Primary] {
		st.Status = Missing
	} else if allEqual(present) {
		st.Status = Match
	} else {
		st.Status = Diff
	}
	r.ByKey[key] = st
	return st
}

func allEqual(vals []string) bool {
	if len(vals) == 0 {
		return false
	}
	for _, v := range vals[1:] {
		if v != vals[0] {
			return false
		}
	}
	return true
}

var formatRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// FormatValid reports whether the key name is well-formed and every present
// value across the selected sources is non-empty.
func FormatValid(st *KeyState) bool {
	if !formatRe.MatchString(st.Key) || len(st.Values) == 0 {
		return false
	}
	for _, v := range st.Values {
		if v == "" {
			return false
		}
	}
	return true
}
