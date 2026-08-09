// Package doctor runs environment health checks and renders a report grouped
// into Environment, Configuration, and Security sections.
package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ldhnam/envigator/internal/audit"
	"github.com/ldhnam/envigator/internal/envfile"
	"github.com/ldhnam/envigator/internal/gitguard"
	"github.com/ldhnam/envigator/internal/lint"
	"github.com/ldhnam/envigator/internal/schema"
	"github.com/ldhnam/envigator/internal/secrets"
)

type Kind int

const (
	OK Kind = iota
	Warn
	Err
)

type Check struct {
	Label  string
	Kind   Kind
	Detail string
}

type Section struct {
	Title  string
	Checks []Check
}

type Report struct {
	Sections []Section
}

// Summary returns the total error and warning counts.
func (r *Report) Summary() (errs, warns int) {
	for _, s := range r.Sections {
		for _, c := range s.Checks {
			switch c.Kind {
			case Err:
				errs++
			case Warn:
				warns++
			}
		}
	}
	return
}

// Run performs the health checks for a project directory.
func Run(dir string) (*Report, error) {
	paths, err := envfile.Discover(dir)
	if err != nil {
		return nil, err
	}
	primary := envfile.Primary(paths)
	git := gitguard.Check(dir, paths)
	rep := &Report{}

	// ---- Environment ----
	env := Section{Title: "Environment"}
	if primary != "" {
		env.Checks = append(env.Checks, Check{Label: ".env exists", Kind: OK})
	} else {
		env.Checks = append(env.Checks, Check{Label: ".env exists", Kind: Err, Detail: "no .env / .env.local found"})
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.example")); err == nil {
		env.Checks = append(env.Checks, Check{Label: ".env.example exists", Kind: OK})
	} else {
		env.Checks = append(env.Checks, Check{Label: ".env.example exists", Kind: Err, Detail: "missing — run 'envigator generate'"})
	}
	if primary != "" {
		switch git.ByPath[primary] {
		case gitguard.Ignored:
			env.Checks = append(env.Checks, Check{Label: ".env is gitignored", Kind: OK})
		case gitguard.Tracked, gitguard.Exposed:
			env.Checks = append(env.Checks, Check{Label: ".env is gitignored", Kind: Err, Detail: "add it to .gitignore"})
		default:
			env.Checks = append(env.Checks, Check{Label: ".env is gitignored", Kind: Warn, Detail: "not a git repository"})
		}
	}
	tracked := 0
	for _, p := range paths {
		if git.ByPath[p] == gitguard.Tracked && !isTemplate(p) {
			tracked++
		}
	}
	switch {
	case tracked > 0:
		env.Checks = append(env.Checks, Check{Label: "No tracked secrets", Kind: Err, Detail: fmt.Sprintf("%d .env file(s) tracked in git", tracked)})
	case !git.IsRepo:
		env.Checks = append(env.Checks, Check{Label: "No tracked secrets", Kind: Warn, Detail: "not a git repository"})
	default:
		env.Checks = append(env.Checks, Check{Label: "No tracked secrets", Kind: OK})
	}
	rep.Sections = append(rep.Sections, env)

	// ---- Configuration ----
	cfg := Section{Title: "Configuration"}
	pv := make(map[string]string)
	if primary != "" {
		if f, perr := envfile.Parse(primary, false); perr == nil {
			pv = f.Values
		}
	}
	lintRep, _ := lint.Scan(dir)
	lintKeys := make(map[string]int)
	for _, iss := range lintRep.Issues {
		if iss.Key != "" {
			lintKeys[iss.Key]++
		}
	}
	audRep, _ := audit.Scan(dir)
	s, _ := schema.Load(dir)

	valid, invalid, unused := 0, 0, 0
	for _, k := range sortedKeys(pv) {
		if lintKeys[k] > 0 {
			invalid++
			continue
		}
		valid++
		if audRep != nil {
			if _, ok := audRep.Usages[k]; !ok {
				unused++
			}
		}
	}
	missing := missingKeys(primary, paths, pv, s)
	cfg.Checks = append(cfg.Checks,
		Check{Label: fmt.Sprintf("%d variable%s valid", valid, plural(valid)), Kind: OK},
		Check{Label: fmt.Sprintf("%d unused variable%s", unused, plural(unused)), Kind: kindFor(unused)},
		Check{Label: fmt.Sprintf("%d missing variable%s", missing, plural(missing)), Kind: kindFor(missing)},
		Check{Label: fmt.Sprintf("%d invalid value%s", invalid, plural(invalid)), Kind: kindFor(invalid)},
	)
	rep.Sections = append(rep.Sections, cfg)

	// ---- Security ----
	sec := Section{Title: "Security"}
	creds := 0
	for _, p := range paths {
		if git.ByPath[p] != gitguard.Tracked {
			continue
		}
		if f, perr := envfile.Parse(p, false); perr == nil {
			for _, k := range f.Keys {
				if len(secrets.Detect(f.Values[k])) > 0 {
					creds++
				}
			}
		}
	}
	switch {
	case creds > 0:
		sec.Checks = append(sec.Checks, Check{Label: "No obvious credentials committed", Kind: Err, Detail: fmt.Sprintf("%d credential(s) in tracked files", creds)})
	case !git.IsRepo:
		sec.Checks = append(sec.Checks, Check{Label: "No obvious credentials committed", Kind: Warn, Detail: "not a git repository"})
	default:
		sec.Checks = append(sec.Checks, Check{Label: "No obvious credentials committed", Kind: OK})
	}

	privFound, privUnignored := false, 0
	for _, p := range paths {
		f, perr := envfile.Parse(p, false)
		if perr != nil {
			continue
		}
		for _, k := range f.Keys {
			for _, mt := range secrets.Detect(f.Values[k]) {
				if mt.Name == "Private Key" {
					privFound = true
					if git.ByPath[p] != gitguard.Ignored {
						privUnignored++
					}
				}
			}
		}
	}
	switch {
	case privFound && privUnignored > 0:
		sec.Checks = append(sec.Checks, Check{Label: "Private keys protected", Kind: Err, Detail: fmt.Sprintf("%d private key(s) in non-ignored files", privUnignored)})
	default:
		sec.Checks = append(sec.Checks, Check{Label: "Private keys protected", Kind: OK})
	}

	backups := backupFiles(dir)
	unIgnoredBackups := 0
	for _, b := range backups {
		if git.ByPath[b] != gitguard.Ignored {
			unIgnoredBackups++
		}
	}
	if unIgnoredBackups > 0 {
		sec.Checks = append(sec.Checks, Check{Label: ".env backups ignored", Kind: Warn, Detail: fmt.Sprintf("%d backup file(s) not gitignored", unIgnoredBackups)})
	} else {
		sec.Checks = append(sec.Checks, Check{Label: ".env backups ignored", Kind: OK})
	}
	rep.Sections = append(rep.Sections, sec)

	return rep, nil
}

func missingKeys(primary string, paths []string, pv map[string]string, s *schema.Schema) int {
	missing := make(map[string]bool)
	if s != nil {
		for name, def := range s.Variables {
			if def.Required && pv[name] == "" {
				missing[name] = true
			}
		}
		return len(missing)
	}
	for _, p := range paths {
		if p == primary || isTemplate(p) {
			continue
		}
		f, err := envfile.Parse(p, false)
		if err != nil {
			continue
		}
		for _, k := range f.Keys {
			if pv[k] == "" {
				missing[k] = true
			}
		}
	}
	return len(missing)
}

func backupFiles(dir string) []string {
	var out []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := strings.ToLower(e.Name())
		backup := strings.HasPrefix(n, ".env") &&
			(strings.Contains(n, "backup") || strings.HasSuffix(n, ".bak") ||
				strings.HasSuffix(n, ".old") || strings.HasSuffix(n, ".orig") ||
				strings.HasSuffix(n, "~"))
		if backup {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

func isTemplate(path string) bool {
	return strings.HasSuffix(strings.ToLower(filepath.Base(path)), ".example")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func kindFor(n int) Kind {
	if n > 0 {
		return Err
	}
	return OK
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// Render formats the report like the reference output.
func Render(r *Report) string {
	var b strings.Builder
	b.WriteString("Envigator Doctor\n")
	for _, s := range r.Sections {
		b.WriteString("\n" + s.Title + "\n")
		b.WriteString(strings.Repeat("─", 36) + "\n\n")
		for _, c := range s.Checks {
			mark := "✓"
			if c.Kind == Warn {
				mark = "⚠"
			}
			if c.Kind == Err {
				mark = "✗"
			}
			b.WriteString(mark + " " + c.Label + "\n")
			if c.Detail != "" {
				b.WriteString("    " + c.Detail + "\n")
			}
		}
	}
	errs, warns := r.Summary()
	b.WriteString("\nSummary\n\n")
	if errs == 1 {
		b.WriteString("  1 error\n")
	} else {
		b.WriteString(fmt.Sprintf("  %d errors\n", errs))
	}
	if warns == 1 {
		b.WriteString("  1 warning\n")
	} else {
		b.WriteString(fmt.Sprintf("  %d warnings\n", warns))
	}
	return b.String()
}
