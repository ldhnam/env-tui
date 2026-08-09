// Package gitguard reports whether .env files are protected from being
// committed by checking Git's ignore rules (via `git check-ignore`) and
// whether they are already tracked in the repository.
package gitguard

import (
	"os/exec"
	"path/filepath"
)

type Status int

const (
	NotRepo Status = iota // directory is not inside a git work tree
	Ignored               // ignored by git and not tracked (safe)
	Exposed               // not ignored (and not tracked) — could be committed
	Tracked               // already tracked by git — ignore rules do not help
)

func (s Status) String() string {
	switch s {
	case NotRepo:
		return "not-a-repo"
	case Ignored:
		return "ignored"
	case Exposed:
		return "exposed"
	case Tracked:
		return "tracked"
	}
	return "unknown"
}

type Report struct {
	IsRepo bool
	ByPath map[string]Status
}

// Check returns the git-ignore status of each path relative to dir.
func Check(dir string, paths []string) *Report {
	rep := &Report{ByPath: make(map[string]Status, len(paths))}
	if !hasGit(dir) {
		rep.IsRepo = false
		for _, p := range paths {
			rep.ByPath[p] = NotRepo
		}
		return rep
	}
	rep.IsRepo = true
	for _, p := range paths {
		rep.ByPath[p] = fileStatus(dir, p)
	}
	return rep
}

func hasGit(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree")
	return cmd.Run() == nil
}

func fileStatus(dir, path string) Status {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return Exposed
	}
	rel = filepath.ToSlash(rel)
	if gitExit("ls-files", "--error-unmatch", "--", rel, dir) == 0 {
		return Tracked
	}
	if gitExit("check-ignore", "-q", "--", rel, dir) == 0 {
		return Ignored
	}
	return Exposed
}

// gitExit runs a git subcommand and returns its exit code (0 on success).
func gitExit(args ...string) int {
	dir := args[len(args)-1]
	cmd := exec.Command("git", append([]string{"-C", dir}, args[:len(args)-1]...)...)
	err := cmd.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}
