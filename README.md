# env-tui

An interactive terminal dashboard for inspecting, diffing, and sanitizing environment variables across local `.env` files, `.env.example` templates, and remote/deployed environment sources — without exposing plaintext secrets on screen by default.

Built with [bubbletea](https://github.com/charmbracelet/bubbletea) and [lipgloss](https://github.com/charmbracelet/lipgloss).

```
env-tui: demo                                             ● Secrets hidden [s]
╭────────────────────╮╭────────────────────────────╮╭────────────────────────────╮
│ Target Files  [x   ││ Keys  [j/k]                ││ Key: DATABASE_URL          │
│ select]            ││ ✓ PORT                     ││ .env.example       : •••••• │
│ [x] .env           ││ ⚠ DATABASE_URL             ││ .env.staging (rem… : •••••• │
│ [x] .env.example   ││ ✗ REDIS_URL                ││ .env.local         : •••••• │
│ [x] .env.local     ││ ✗ STRIPE_WEBHOOK_KEY       ││ Status : DIFF (Format Valid)│
│ [x] .env.staging   ││ ⚠ API_KEY                  ││ Values differ across sources │
│ (remote)           ││ …                          ││                             │
╰────────────────────╯╰────────────────────────────╯╰────────────────────────────╯
╭────────────────────────────────────────────────────────────────────────────────╮
│ Missing Keys in .env.local (4):                                                │
│   REDIS_URL                    Present in .env, .env.example, .env.staging     │
│   STRIPE_WEBHOOK_KEY           Present in .env.example, .env.staging          │
│   [a] autofill  [c] copy value  (tab → missing)                                │
╰────────────────────────────────────────────────────────────────────────────────╯
j/k nav · tab focus · s secrets · x select · a autofill · c copy · r reload · ? help · q quit
```

## Why

Developers constantly juggle `.env` files across microservices and git branches. Keys get added in production or staging, leaving local files broken with missing variables, exposed secrets, or out-of-sync `.env.example` templates. env-tui gives you a side-by-side view of every environment source so you can spot gaps at a glance.

## Features

- **Side-by-side diffing** — visually highlights keys that are missing, differing, or in sync across `.env`, `.env.local`, `.env.example`, and remote vault files (`.env.staging`, `.env.production`, …).
- **Obfuscation mode** — sensitive values are masked as `••••••` until you press `s`.
- **Interactive sync** — press `a` on a missing key to pull its name into your local `.env` with a placeholder prompt; press `c` to copy a value straight to the clipboard.
- **Remote-aware** — files like `.env.staging` are tagged `(remote)` and treated as deployed environments. The same source model can be wired to secret managers (Doppler, Infisical, AWS SSM, …).

## Install

```sh
go install github.com/ldhnam/env-tui@latest
```

Or run directly from source:

```sh
go run . <directory>
```

## Usage

```sh
env-tui [directory]     # defaults to the current directory
```

Point it at a project directory containing `.env*` files:

```sh
env-tui ~/projects/payment-service
```

### Keybindings

| Key                 | Action                                   |
| ------------------- | ---------------------------------------- |
| `j` / `k`, `↑` / `↓` | move within the focused panel           |
| `tab`, `←` / `→`     | cycle focus: files → keys → missing     |
| `s`                 | toggle secret obfuscation                |
| `x`                 | toggle whether a source file is included |
| `a`                 | autofill the selected missing key into the primary `.env` |
| `c`                 | copy the selected key's value to the clipboard |
| `r`                 | rescan the directory                     |
| `g` / `G`           | jump to top / bottom                     |
| `?`                 | toggle help                              |
| `q`                 | quit                                     |

### Panels

1. **Target Files** — checkbox list of every discovered `.env*` source. `.env`-local variants are `(local)`; deployment files are tagged `(remote)`.
2. **Keys** — union of all keys across selected sources with a status glyph: `✓` MATCH, `⚠` DIFF, `✗` MISSING.
3. **Detail** — the focused key's value in each selected source (masked by default), its aggregate status, and a `Format Valid` hint.
4. **Missing Keys** — keys absent from the primary local file (`.env.local` preferred, else `.env`), with the sources that do define them. `a` autofills, `c` copies.

### Which file is "primary"?

The reference for the **Missing Keys** panel is `.env.local` when present, otherwise `.env`, otherwise the first discovered file. Autofill (`a`) appends the new key to that primary file.

## Remote secret managers

The core operates on a unified `envfile.File` source model (`Path`, `Name`, `Remote`, `Keys`, `Values`). A remote backend (Doppler / Infisical / AWS) can be implemented as an additional source: fetch key/value pairs, mark `Remote = true`, and return it from discovery alongside local files. The diff, obfuscation, and sync features then work unchanged.

## Development

```sh
go build ./... && go vet ./... && go test ./...
```

A sample project lives in `demo/` for quick exploration:

```sh
go run . demo
```
