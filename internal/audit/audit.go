// Package audit scans project source files for environment variable
// references (e.g. process.env.PORT, os.Getenv("DATABASE_URL"), $VAR)
// using per-language access patterns.
package audit

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

const maxFileSize = 5 << 20 // 5 MiB

type Usage struct {
	Key   string
	Count int              // unique (file, line) references
	Files map[string][]int // file path -> line numbers
}

func (u *Usage) add(file string, line int) {
	if slices.Contains(u.Files[file], line) {
		return
	}
	u.Count++
	if u.Files == nil {
		u.Files = make(map[string][]int)
	}
	u.Files[file] = append(u.Files[file], line)
}

type Report struct {
	Files  int               // files scanned
	Refs   int               // total unique references found
	Usages map[string]*Usage // key -> usage
}

func (rep *Report) add(path, key string, line int) {
	u := rep.Usages[key]
	if u == nil {
		u = &Usage{Key: key}
		rep.Usages[key] = u
	}
	u.add(path, line)
	rep.Refs++
}

// Sorted returns usages ordered by key.
func (rep *Report) Sorted() []*Usage {
	out := make([]*Usage, 0, len(rep.Usages))
	for _, u := range rep.Usages {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

type rule struct {
	exts  []string
	bases []string
	re    *regexp.Regexp
}

func (r *rule) matches(lowerBase string) bool {
	if slices.Contains(r.bases, lowerBase) {
		return true
	}
	for _, e := range r.exts {
		if strings.HasSuffix(lowerBase, e) {
			return true
		}
	}
	return false
}

var name = `[A-Za-z_][A-Za-z0-9_]*`
var jsExts = []string{".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".vue", ".svelte"}

var allRules = []rule{
	// --- JavaScript / TypeScript ---
	{exts: jsExts, re: regexp.MustCompile(`process\.env\.(?P<name>` + name + `)`)},
	{exts: jsExts, re: regexp.MustCompile(`process\.env\s*\[\s*["'](?P<name>` + name + `)["']\s*\]`)},
	{exts: jsExts, re: regexp.MustCompile(`import\.meta\.env\.(?P<name>` + name + `)`)},
	{exts: jsExts, re: regexp.MustCompile(`import\.meta\.env\s*\[\s*["'](?P<name>` + name + `)["']\s*\]`)},
	{exts: jsExts, re: regexp.MustCompile(`(?:^|\n)\s*(?:const|let|var)\s*\{\s*(?P<name>` + name + `)`)},                // destructuring, first id
	{exts: jsExts, re: regexp.MustCompile(`,\s*(?P<name>` + name + `)\s*(?:,|\})\s*=\s*(?:process|import\.meta)\.env`)}, // destructuring, rest

	// --- Python ---
	{exts: []string{".py"}, re: regexp.MustCompile(`os\.environ\s*\[\s*["'](?P<name>` + name + `)["']\s*\]`)},
	{exts: []string{".py"}, re: regexp.MustCompile(`os\.environ\.get\s*\(\s*["'](?P<name>` + name + `)["']`)},
	{exts: []string{".py"}, re: regexp.MustCompile(`os\.getenv\s*\(\s*["'](?P<name>` + name + `)["']`)},

	// --- Go ---
	{exts: []string{".go"}, re: regexp.MustCompile(`(?:os\.)?(?:Getenv|LookupEnv)\s*\(\s*["` + "`" + `](?P<name>` + name + `)["` + "`" + `]`)},

	// --- Rust ---
	{exts: []string{".rs"}, re: regexp.MustCompile(`env::var(?:_os)?\s*\(\s*"(?P<name>` + name + `)"`)},
	{exts: []string{".rs"}, re: regexp.MustCompile(`(?:std::)?env!\s*\(\s*"(?P<name>` + name + `)"`)},

	// --- PHP ---
	{exts: []string{".php"}, re: regexp.MustCompile(`getenv\s*\(\s*["'](?P<name>` + name + `)["']`)},
	{exts: []string{".php"}, re: regexp.MustCompile(`\$_ENV\s*\[\s*["'](?P<name>` + name + `)["']`)},
	{exts: []string{".php"}, re: regexp.MustCompile(`\$_SERVER\s*\[\s*["'](?P<name>` + name + `)["']`)},

	// --- Ruby ---
	{exts: []string{".rb"}, re: regexp.MustCompile(`ENV\.(?:fetch|delete|\[\])\s*\(\s*["'](?P<name>` + name + `)["']`)},
	{exts: []string{".rb"}, re: regexp.MustCompile(`ENV\s*\[\s*["'](?P<name>` + name + `)["']\s*\]`)},

	// --- Java / Kotlin ---
	{exts: []string{".java", ".kt", ".kts"}, re: regexp.MustCompile(`System\.getenv\s*\(\s*"(?P<name>` + name + `)"`)},

	// --- C / C++ ---
	{exts: []string{".c", ".cc", ".cpp", ".cxx", ".h", ".hpp"}, re: regexp.MustCompile(`(?:std::)?getenv\s*\(\s*"(?P<name>` + name + `)"`)},

	// --- C# ---
	{exts: []string{".cs"}, re: regexp.MustCompile(`Environment\.GetEnvironmentVariable\s*\(\s*"(?P<name>` + name + `)"`)},

	// --- Swift ---
	{exts: []string{".swift"}, re: regexp.MustCompile(`ProcessInfo\.processInfo\.environment\s*\[\s*"(?P<name>` + name + `)"`)},

	// --- Shell / config files ---
	{exts: []string{".sh", ".bash", ".zsh", ".ksh", ".yaml", ".yml", ".toml", ".json", ".mk"}, bases: []string{"dockerfile", "makefile"}, re: regexp.MustCompile(`\$\{(?P<name>` + name + `)(?:\s*[:+-].*)?\}`)},
	{exts: []string{".sh", ".bash", ".zsh", ".ksh"}, re: regexp.MustCompile(`\$(?P<name>` + name + `)\b`)},
	{exts: []string{".ps1"}, re: regexp.MustCompile(`\$env\s*[:]\s*(?P<name>` + name + `)`)},
}

var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, ".next": true, ".nuxt": true, "__pycache__": true,
	".venv": true, "venv": true, "coverage": true, ".terraform": true,
	".cache": true, "tmp": true, "out": true, "bin": true, "obj": true,
	".git": true, ".svn": true, ".hg": true, ".idea": true, ".vscode": true,
	"Pods": true, ".gradle": true, ".tox": true, ".nox": true,
}

func shouldSkipDir(root, path string, d fs.DirEntry) bool {
	name := d.Name()
	if path != root && strings.HasPrefix(name, ".") {
		return true
	}
	return skipDirs[name]
}

// Scan walks root and collects env var usage across supported source files.
func Scan(root string) (*Report, error) {
	rep := &Report{Usages: make(map[string]*Usage)}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(root, path, d) {
				return filepath.SkipDir
			}
			return nil
		}
		scanFile(path, rep)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rep, nil
}

func scanFile(path string, rep *Report) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxFileSize {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lowerBase := strings.ToLower(filepath.Base(path))
	var rules []*rule
	for i := range allRules {
		if allRules[i].matches(lowerBase) {
			rules = append(rules, &allRules[i])
		}
	}
	if len(rules) == 0 {
		return
	}
	rep.Files++
	content := string(data)
	for li, line := range strings.Split(content, "\n") {
		for _, r := range rules {
			idx := r.re.SubexpIndex("name")
			if idx < 0 {
				continue
			}
			for _, m := range r.re.FindAllStringSubmatch(line, -1) {
				if m[idx] != "" {
					rep.add(path, m[idx], li+1)
				}
			}
		}
	}
}
