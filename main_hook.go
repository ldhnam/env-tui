package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ldhnam/envigator/internal/audit"
	"github.com/ldhnam/envigator/internal/envfile"
	"github.com/ldhnam/envigator/internal/secrets"
)

// cliHook implements `envigator hook install` and `envigator hook check`.
func cliHook(args []string) {
	sub := "install"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = args[0]
		args = args[1:]
	}
	switch sub {
	case "install":
		fs := flag.NewFlagSet("hook install", flag.ExitOnError)
		preCommit := fs.Bool("pre-commit", false, "install only the pre-commit hook")
		postCheckout := fs.Bool("post-checkout", false, "install only the post-checkout hook")
		_ = fs.Parse(args)
		installHooks(*preCommit, *postCheckout)
	case "check":
		checkHookArgs(args)
	default:
		fmt.Fprintln(os.Stderr, "envigator hook: unknown subcommand (install|check)")
		os.Exit(1)
	}
}

func gitDir() string {
	out, err := exec.Command("git", "rev-parse", "--git-dir").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "envigator hook: not a git repository")
		os.Exit(1)
	}
	return strings.TrimSpace(string(out))
}

func installHooks(preOnly, postOnly bool) {
	gd := gitDir()
	exe, _ := os.Executable()
	writeHook := func(name, body string) {
		path := filepath.Join(gd, "hooks", name)
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "envigator hook: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("installed %s\n", path)
	}
	locate := fmt.Sprintf(`ENVIGATOR_BIN="%s"
if [ ! -x "$ENVIGATOR_BIN" ]; then
  ENVIGATOR_BIN="$(command -v envigator 2>/dev/null)"
fi
if [ -z "$ENVIGATOR_BIN" ]; then
  echo "envigator: not found — skipping check" >&2
  exit 0
fi
`, exe)
	if !postOnly {
		writeHook("pre-commit", `#!/bin/sh
# envigator: block commits containing unencrypted credentials
`+locate+`"$ENVIGATOR_BIN" hook check --staged || exit 1
`)
	}
	if !preOnly {
		writeHook("post-checkout", `#!/bin/sh
# envigator: warn when a branch switch introduces ghost keys
`+locate+`"$ENVIGATOR_BIN" hook check --ghosts
exit 0
`)
	}
}

func checkHookArgs(args []string) {
	staged, ghosts := false, false
	for _, a := range args {
		switch a {
		case "--staged":
			staged = true
		case "--ghosts":
			ghosts = true
		}
	}
	switch {
	case staged:
		checkStagedSecrets()
	case ghosts:
		checkGhosts()
	default:
		fmt.Fprintln(os.Stderr, "envigator hook check: use --staged or --ghosts")
		os.Exit(1)
	}
}

// checkStagedSecrets scans staged .env files for credential patterns and
// exits non-zero if any are found (blocking the commit).
func checkStagedSecrets() {
	out, err := exec.Command("git", "diff", "--cached", "--name-only", "--diff-filter=ACM").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "envigator hook: %v\n", err)
		os.Exit(1)
	}
	blocked := false
	for _, name := range strings.Fields(string(out)) {
		if !strings.HasPrefix(filepath.Base(name), ".env") {
			continue
		}
		blob, err := exec.Command("git", "show", ":"+name).Output()
		if err != nil {
			continue
		}
		f, err := envfile.ParseContent(string(blob), name, false)
		if err != nil {
			continue
		}
		for _, k := range f.Keys {
			if matches := secrets.Detect(f.Values[k]); len(matches) > 0 {
				fmt.Fprintf(os.Stderr, "envigator: blocked commit — %s defines %s (%s)\n", name, k, matches[0].Name)
				blocked = true
			}
		}
	}
	if blocked {
		fmt.Fprintln(os.Stderr, "envigator: unencrypted credentials staged; refusing to commit")
		os.Exit(1)
	}
}

// checkGhosts warns about keys referenced in code but missing from every
// .env file after a branch switch. Does not block.
func checkGhosts() {
	rootOut, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "envigator hook: not a git repository")
		os.Exit(1)
	}
	root := strings.TrimSpace(string(rootOut))
	rep, err := audit.Scan(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "envigator hook: %v\n", err)
		os.Exit(1)
	}
	paths, _ := envfile.Discover(root)
	defined := make(map[string]bool)
	for _, p := range paths {
		if f, err := envfile.Parse(p, false); err == nil {
			for k := range f.Values {
				defined[k] = true
			}
		}
	}
	for _, u := range rep.Sorted() {
		if !defined[u.Key] {
			fmt.Fprintf(os.Stderr, "envigator: warning — %s is used in code but missing from every .env file\n", u.Key)
		}
	}
}
