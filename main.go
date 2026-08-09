package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ldhnam/envigator/internal/tui"
)

func main() {
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
