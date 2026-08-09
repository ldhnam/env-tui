package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// workspace is a directory containing .env files, optionally inside a
// monorepo workspace (pnpm, turbo, go, lerna, npm).
type workspace struct {
	path   string
	marker string
	envs   int
}

var workspaceMarkers = map[string]string{
	"pnpm-workspace.yaml": "pnpm",
	"turbo.json":          "turbo",
	"go.work":             "go",
	"lerna.json":          "lerna",
	"package.json":        "npm",
}

// findWorkspaces returns the root plus nested directories (up to maxDepth)
// that contain .env files, so monorepo contexts can be switched between.
func findWorkspaces(root string, maxDepth int) []workspace {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	rootWS := scanWorkspace(rootAbs)
	if rootWS.marker == "" {
		rootWS.marker = "root"
	}
	out := []workspace{rootWS}
	_ = filepath.WalkDir(rootAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path == rootAbs {
			return nil
		}
		rel, _ := filepath.Rel(rootAbs, path)
		depth := len(strings.Split(rel, string(filepath.Separator)))
		if depth > maxDepth {
			return filepath.SkipDir
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == ".git" {
			return filepath.SkipDir
		}
		ws := scanWorkspace(path)
		if ws.envs == 0 {
			return nil
		}
		out = append(out, ws)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// scanWorkspace counts .env files and detects a monorepo marker in dir.
func scanWorkspace(dir string) workspace {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return workspace{path: dir}
	}
	ws := workspace{path: dir}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".env") {
			ws.envs++
		}
		if m, ok := workspaceMarkers[e.Name()]; ok && ws.marker == "" {
			ws.marker = m
		}
	}
	return ws
}
