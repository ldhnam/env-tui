package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/ldhnam/envigator/internal/envfile"
	"github.com/ldhnam/envigator/internal/snapshot"
	"github.com/ldhnam/envigator/internal/tui"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "run":
			cliRun(os.Args[2:])
			return
		case "generate":
			cliGenerate(os.Args[2:])
			return
		case "snapshot":
			cliSnapshot(os.Args[2:])
			return
		case "help", "-h", "--help":
			printUsage()
			return
		}
	}
	runTUI()
}

func printUsage() {
	fmt.Print(`envigator — inspect, diff, audit, and guard .env files across environments.

Usage:
  envigator [flags] [directory]      launch the TUI (directory defaults to ".")
  envigator run [--dir DIR] -- cmd   run a command with the loaded .env in-memory
  envigator generate [dir]           write a sanitized .env.example from the primary .env
  envigator snapshot <create|list|restore|delete> [flags]
      --dir DIR          target directory (default ".")
      --passphrase S     encryption passphrase (or set ENVIGATOR_PASSPHRASE)
      --name NAME        snapshot to restore/delete

Flags:
  -vault string        secret manager provider (doppler, vault, op, aws, infisical)
  -vault-project string  provider project / item / kv path / region
  -vault-env string      provider environment / config / vault
`)
}

func runTUI() {
	var opts tui.Options
	flag.StringVar(&opts.VaultProvider, "vault", "", "secret manager provider (doppler, vault, op, aws, infisical)")
	flag.StringVar(&opts.VaultProject, "vault-project", "", "provider project / item / kv path / region")
	flag.StringVar(&opts.VaultEnv, "vault-env", "", "provider environment / config / vault")
	flag.Parse()

	dir := "."
	if args := flag.Args(); len(args) > 0 {
		dir = args[0]
	}

	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		fmt.Fprintf(os.Stderr, "envigator: %q is not a directory\n", dir)
		os.Exit(1)
	}
	p := tea.NewProgram(tui.New(dir, opts), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "envigator:", err)
		os.Exit(1)
	}
}

// cliSnapshot manages encrypted .env snapshots from the command line.
func cliSnapshot(args []string) {
	sub := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("snapshot "+sub, flag.ExitOnError)
	dir := fs.String("dir", ".", "directory containing .env files")
	pass := fs.String("passphrase", "", "passphrase (or set ENVIGATOR_PASSPHRASE)")
	name := fs.String("name", "", "snapshot name (restore/delete)")
	_ = fs.Parse(args)

	if *pass == "" {
		*pass = os.Getenv("ENVIGATOR_PASSPHRASE")
	}
	if sub == "create" || sub == "restore" {
		if *pass == "" {
			p, err := promptPassphrase()
			if err != nil {
				fmt.Fprintln(os.Stderr, "envigator snapshot:", err)
				os.Exit(1)
			}
			*pass = p
		}
	}

	switch sub {
	case "create":
		paths, err := envfile.Discover(*dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "envigator snapshot: %v\n", err)
			os.Exit(1)
		}
		files := make(map[string]string)
		for _, p := range paths {
			if data, err := os.ReadFile(p); err == nil {
				files[filepath.Base(p)] = string(data)
			}
		}
		n, err := snapshot.Create(*dir, *pass, files)
		if err != nil {
			fmt.Fprintf(os.Stderr, "envigator snapshot: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("created %s (%d files)\n", n, len(files))
	case "list":
		list, err := snapshot.List(*dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "envigator snapshot: %v\n", err)
			os.Exit(1)
		}
		for _, n := range list {
			fmt.Println(n)
		}
	case "restore":
		if *name == "" {
			fmt.Fprintln(os.Stderr, "envigator snapshot: --name required to restore")
			os.Exit(1)
		}
		files, err := snapshot.Read(*dir, *name, *pass)
		if err != nil {
			fmt.Fprintf(os.Stderr, "envigator snapshot: %v\n", err)
			os.Exit(1)
		}
		n := 0
		for fname, content := range files {
			if err := os.WriteFile(filepath.Join(*dir, fname), []byte(content), 0o644); err == nil {
				n++
			}
		}
		fmt.Printf("restored %d file(s) from %s\n", n, *name)
	case "delete":
		if *name == "" {
			fmt.Fprintln(os.Stderr, "envigator snapshot: --name required to delete")
			os.Exit(1)
		}
		if err := snapshot.Delete(*dir, *name); err != nil {
			fmt.Fprintf(os.Stderr, "envigator snapshot: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("deleted %s\n", *name)
	default:
		fmt.Fprintln(os.Stderr, "envigator snapshot: unknown subcommand (create|list|restore|delete)")
		os.Exit(1)
	}
}

// promptPassphrase reads a passphrase from the terminal without echo.
func promptPassphrase() (string, error) {
	fmt.Fprint(os.Stderr, "passphrase: ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), err
}

// cliGenerate writes a sanitized .env.example for the target directory.
func cliGenerate(args []string) {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	content, err := envfile.BuildExampleTemplate(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "envigator generate: %v\n", err)
		os.Exit(1)
	}
	if content == "" {
		fmt.Fprintln(os.Stderr, "envigator generate: no .env files found")
		os.Exit(1)
	}
	path := filepath.Join(dir, ".env.example")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "envigator generate: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("generated %s\n", path)
}

// cliRun runs a command with the loaded environment profile applied to the
// child process only — the .env file on disk is never modified.
func cliRun(args []string) {
	dir := "."
	var cmdArgs []string
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep >= 0 {
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		fs.StringVar(&dir, "dir", ".", "directory containing .env files")
		_ = fs.Parse(args[:sep])
		cmdArgs = args[sep+1:]
	} else {
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		fs.StringVar(&dir, "dir", ".", "directory containing .env files")
		_ = fs.Parse(args)
	}
	if len(cmdArgs) == 0 {
		fmt.Fprintln(os.Stderr, "envigator run: no command given (separate it with --)")
		os.Exit(1)
	}
	env, err := envfile.LoadForRun(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "envigator run: %v\n", err)
		os.Exit(1)
	}
	childEnv := make([]string, 0, len(os.Environ())+len(env))
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.Index(kv, "="); i >= 0 {
			key = kv[:i]
		}
		if _, ok := env[key]; ok {
			continue
		}
		childEnv = append(childEnv, kv)
	}
	for k, v := range env {
		childEnv = append(childEnv, k+"="+v)
	}

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = childEnv
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "envigator run:", err)
		os.Exit(1)
	}
}
