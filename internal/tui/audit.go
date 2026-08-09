package tui

import (
	"sort"

	"github.com/ldhnam/envigator/internal/audit"
)

// displayItem is an entry in the main keys list: either an environment key
// (with a diff state) or a ghost key (referenced in code but present in no
// .env file).
type displayItem struct {
	key   string
	ghost bool
}

// displayKeys returns the ordered key list shown in the Keys panel:
// ghost keys first (most critical), then environment keys in diff order.
func (m Model) displayKeys() []displayItem {
	var items []displayItem
	if m.rep == nil {
		return items
	}
	for _, g := range m.ghostKeys() {
		items = append(items, displayItem{key: g.Key, ghost: true})
	}
	for _, st := range m.rep.All {
		items = append(items, displayItem{key: st.Key})
	}
	return items
}

// inventoryKeys is the set of keys defined across every discovered .env file,
// selected or not.
func (m Model) inventoryKeys() map[string]bool {
	s := make(map[string]bool)
	for _, f := range m.files {
		for k := range f.Values {
			s[k] = true
		}
	}
	return s
}

// primaryKeys is the set of keys defined in the primary local file.
func (m Model) primaryKeys() map[string]bool {
	s := make(map[string]bool)
	for _, f := range m.files {
		if f.Path == m.prim {
			for k := range f.Values {
				s[k] = true
			}
		}
	}
	return s
}

// ghostKeys are referenced in code but absent from every .env file.
func (m Model) ghostKeys() []*audit.Usage {
	var out []*audit.Usage
	if m.audit == nil {
		return out
	}
	inv := m.inventoryKeys()
	for _, u := range m.audit.Usages {
		if !inv[u.Key] {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// zombieKeys are defined in some .env file but never referenced in code.
func (m Model) zombieKeys() []string {
	var out []string
	if m.audit == nil {
		return out
	}
	inv := m.inventoryKeys()
	for k := range inv {
		if _, ok := m.audit.Usages[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// usedMissingPrimary are referenced in code, present in at least one .env
// file, but missing from the primary local file.
func (m Model) usedMissingPrimary() []*audit.Usage {
	var out []*audit.Usage
	if m.audit == nil {
		return out
	}
	inv := m.inventoryKeys()
	prim := m.primaryKeys()
	for _, u := range m.audit.Usages {
		if inv[u.Key] && !prim[u.Key] {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// isZombie reports whether key exists in some .env file but has no code refs.
func (m Model) isZombie(key string) bool {
	if m.audit == nil {
		return false
	}
	if _, ok := m.audit.Usages[key]; ok {
		return false
	}
	return m.inventoryKeys()[key]
}

func (m Model) keyGlyph(it displayItem) string {
	if it.ghost {
		return missStyle.Render("✗")
	}
	st := m.rep.ByKey[it.key]
	if st == nil {
		return "?"
	}
	return statusGlyph(st)
}
