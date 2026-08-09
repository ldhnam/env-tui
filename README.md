# envigator

> An interactive terminal dashboard for inspecting, diffing, and sanitizing `.env` files across environments — without leaking secrets on screen.

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![CI](https://img.shields.io/github/actions/workflow/status/ldhnam/envigator/ci.yml?branch=main)](https://github.com/ldhnam/envigator/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/ldhnam/envigator)](https://goreportcard.com/report/github.com/ldhnam/envigator)
[![Release](https://img.shields.io/github/v/release/ldhnam/envigator?sort=semver)](https://github.com/ldhnam/envigator/releases)

**envigator** is a terminal UI for developers who juggle `.env` files across microservices, branches, and environments. It diffs your local files against templates and remote/deployed environments, audits which variables your code actually uses, lints file formatting, and guards against accidentally committing real secrets — all without printing plaintext values by default.

Built with [bubbletea](https://github.com/charmbracelet/bubbletea) and [lipgloss](https://github.com/charmbracelet/lipgloss).

## Highlights

- 🔎 **Side-by-side diffing** across `.env`, `.env.local`, `.env.example`, and remote sources — see `MATCH` / `DIFF` / `MISSING` at a glance
- 👻 **Ghost & zombie keys** — code refs that exist in no `.env` file, and env keys nothing references
- 🧹 **Format & naming lint** — `UPPER_SNAKE_CASE`, syntax, whitespace, duplicates, empty values
- 🚨 **Leak detector & pre-commit guard** — flags Stripe / AWS / OpenAI / GitHub token patterns; autofill requires `y`/`n` before saving a secret-like value
- 🕶️ **Stealth mode** — values masked as `••••••••`; reveal all (`s`), one key (`space`), or hover with the mouse
- 🛡️ **Safety Check** — a header badge reports whether `.env`/secret files are git-ignored, with per-file markers in Target Files (`✓` ignored · `!` not ignored · `T` tracked · `–` not a git repo)
- ✏️ **In-Place Editor** — press `e` to edit the focused key's value in a multi-line editor (ideal for RSA private keys, JSON payloads); `ctrl+s` saves in place, `esc` cancels, and multi-line values are stored as quoted `\n`-escaped strings that round-trip cleanly
- 📋 **One-Click Clipboard Exports** — copy `export KEY="VALUE"` lines or full blocks formatted for **Bash / Zsh / Fish** (`T` cycles the target shell), plus copy key names or values individually: `c` value, `C` name, `E` export line, `B` export block
- ⚡ **Fast** — pattern-based scanning across JS, TS, Go, Python, Rust, PHP, Ruby, shell and more, runs async

```
envigator: demo                                             ● Secrets hidden [s]
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

Developers constantly juggle `.env` files across microservices and git branches. Keys get added in production or staging, leaving local files broken with missing variables, exposed secrets, or out-of-sync `.env.example` templates. envigator gives you a side-by-side view of every environment source so you can spot gaps at a glance.

## Features

- **Side-by-side diffing** — visually highlights keys that are missing, differing, or in sync across `.env`, `.env.local`, `.env.example`, and remote vault files (`.env.staging`, `.env.production`, …).
- **Code Audit** — scans project source files (`.js`, `.ts`, `.py`, `.go`, `.rs`, `.php`, `.rb`, `.sh`, …) for env var usage (`process.env.PORT`, `os.Getenv("DATABASE_URL")`, `$VAR`, …) and cross-references it with your environment files to detect:
  - **Ghost keys** — referenced in code but missing from *all* `.env` files (runtime breaks waiting to happen).
  - **Used-but-missing** — referenced in code and present elsewhere, but not in your local `.env`.
  - **Zombie keys** — defined in `.env` files but never referenced anywhere in the source code.
- **Format & Naming Lint** — flags non-`UPPER_SNAKE_CASE` keys, malformed `KEY=VALUE` syntax (missing `=`, unclosed quotes), accidental whitespace (around `=`, in values, leading indentation), duplicate keys, and empty values — with per-line findings grouped by file.
- **Pre-Commit Guard & Leak Detector** — values that look like unencrypted credentials (Stripe, AWS, OpenAI, GitHub, Slack, Google, JWT, private keys, …) are detected and surfaced as `S` markers in the Keys list, flagged in the detail pane, and listed in the Lint panel. When autofill writes a secret-like value, a `y`/`n` confirmation is required before it hits the `.env` file.
- **Stealth/Masking mode** — sensitive values are masked as `••••••••` by default. Press `s` to reveal everything, `space` to reveal just the focused key, or **hover a key with the mouse** to peek at it (hover also selects the key).
- **Interactive sync** — press `a` on a missing key to pull its name into your local `.env` with a placeholder prompt; press `c` to copy a value straight to the clipboard.
- **Remote-aware** — files like `.env.staging` are tagged `(remote)` and treated as deployed environments. The same source model can be wired to secret managers (Doppler, Infisical, AWS SSM, …).

## Install

```sh
go install github.com/ldhnam/envigator@latest
```

Or grab the [latest release binary](https://github.com/ldhnam/envigator/releases) for your platform.

Or run directly from source:

```sh
go run github.com/ldhnam/envigator@latest <directory>
```

## Usage

```sh
envigator [directory]     # defaults to the current directory
```

Point it at a project directory containing `.env*` files:

```sh
envigator ~/projects/payment-service
```

### Keybindings

| Key                 | Action                                   |
| ------------------- | ---------------------------------------- |
| `j` / `k`, `↑` / `↓` | move within the focused panel           |
| `tab`, `←` / `→`     | cycle focus: files → keys → missing     |
| `s`                 | toggle secret obfuscation (reveal/hide all)     |
| `space`             | reveal the focused key's values                 |
| `mouse`             | hover a key to reveal it (selects on hover)     |
| `x`                 | toggle whether a source file is included |
| `a`                 | autofill the selected missing key into the primary `.env` |
| `e`                 | edit the focused key in place (multi-line editor)   |
| `c` / `C`           | copy key value / key name                |
| `E`                 | copy `export KEY="VALUE"` line for the focused key |
| `B`                 | copy an export block for all keys in the primary `.env` |
| `T`                 | cycle export shell format (bash / zsh / fish) |
| `v`                 | toggle the Code Audit panel                     |
| `f`                 | toggle the Format & Naming Lint panel           |
| `r`                 | rescan the directory + re-audit source code      |
| `g` / `G`           | jump to top / bottom                     |
| `?`                 | toggle help                              |
| `q`                 | quit                                     |

### Panels

1. **Target Files** — checkbox list of every discovered `.env*` source. `.env`-local variants are `(local)`; deployment files are tagged `(remote)`. Each file also carries a git-safety glyph (`✓` ignored, `!` not ignored, `T` tracked in git, `–` not a repo) and a lint issue badge; `.env.example` templates are exempt from git warnings since they're meant to be committed.
   - The header shows an overall **Safety Check** badge: `git: protected`, `git: N exposed`, or `git: n/a` when not inside a git work tree. Ignore status is resolved with `git check-ignore` and covers `.gitignore`, `.git/info/exclude`, and negation rules — files already tracked are flagged as `T` since ignore rules can't protect them.
2. **Keys** — union of all keys across selected sources with a status glyph: `✓` MATCH, `⚠` DIFF, `✗` MISSING.
3. **Detail** — the focused key's value in each selected source (masked by default; `space` or hover to reveal), its aggregate status, a `Format Valid` hint, and how many times it's referenced in code. Press `e` to edit the value in place.
4. **Missing Keys** — keys absent from the primary local file (`.env.local` preferred, else `.env`), with the sources that do define them. `a` autofills, `c` copies; keys referenced in code are flagged `[used ×N]`.
5. **Code Audit** (`v`) — cross-references source usage with the environment inventory:
   - **Ghost Keys** — referenced in code but missing from every `.env` file. Also surfaced at the top of the **Keys** panel (marked `✗`) so they're browsable and navigable.
   - **Used but missing from the primary env** — present in other sources but absent locally.
   - **Zombie Keys** — defined in `.env` files but never referenced in code (marked `z` in the Keys list). Candidates for cleanup.
6. **Format & Naming Lint + Leak Detector** (`f`) — per-file list of issues with line numbers and kinds:
   - **Secrets Detected** — keys whose values match known credential formats (potential leaks).
   - `bad-name` — key is not `UPPER_SNAKE_CASE` (e.g. `databaseUrl`, `MY-KEY`, `123KEY`).
   - `whitespace` — accidental spaces around `=`, leading indentation, or leading/trailing spaces in values.
   - `syntax` — missing `=` (expected `KEY=VALUE`), empty key name, unclosed quotes.
   - `duplicate` — the same key defined more than once in a file.
   - `empty-value` — key present with no value.
   Affected files get a `⚠N` badge in **Target Files**, and flagged keys carry a `⚠N` marker in the **Keys** panel and detail view. Secret-bearing keys carry an `S` marker, and autofill values matching a credential pattern are gated behind a `y`/`n` confirmation (Pre-Commit Guard).

## Code Audit coverage

The auditor uses fast, per-language pattern detection (no full AST, so it works across many languages in one pass):

| Pattern | Languages |
| --- | --- |
| `process.env.X`, `process.env["X"]`, `import.meta.env.X` | JS, TS, Vue, Svelte |
| `os.environ["X"]`, `os.environ.get("X")`, `os.getenv("X")` | Python |
| `os.Getenv("X")`, `os.LookupEnv("X")` | Go |
| `env::var("X")`, `env!("X")` | Rust |
| `getenv("X")`, `$_ENV["X"]`, `$_SERVER["X"]` | PHP |
| `ENV["X"]`, `ENV.fetch("X")` | Ruby |
| `System.getenv("X")` | Java, Kotlin |
| `getenv("X")` | C, C++ |
| `Environment.GetEnvironmentVariable("X")` | C# |
| `ProcessInfo.processInfo.environment["X"]` | Swift |
| `$X`, `${X}`, `${X:-default}`, `$env:X` | Shell, Dockerfile, Makefile, YAML |

It also detects JS destructuring (`const { PORT, DB } = process.env;`). It skips hidden dirs, `node_modules`, `vendor`, `dist`, `build`, `target`, etc., and never scans `.env*` files themselves. Ghost/zombie classification considers **all** discovered `.env*` files (selected or not) as the environment inventory.

## Leak detector coverage

Values are scanned against known credential formats:

| Detector | Pattern |
| --- | --- |
| Stripe | `sk_live_`, `sk_test_`, `rk_*`, `pk_*` |
| AWS | `AKIA…`/`ASIA…` access keys, session tokens |
| OpenAI | `sk-…` |
| GitHub | `ghp_`/`gho_`/`ghu_`/`ghs_`/`ghr_`, `github_pat_` |
| Slack | `xoxb-`, `xoxp-`, `xoxa-`, `xoxr-` |
| Google | `AIza…` API keys, `GOCSPX-` OAuth secrets |
| SendGrid / Twilio / npm / Heroku | `SG.`, `SK…`, `npm_`, `heroku_` |
| JWT / Bearer | `eyJ…`, `Bearer …` |
| Private keys | `-----BEGIN …PRIVATE KEY-----` |

Detected values are never printed in plaintext — the guard and panels show only the pattern name and a masked prefix.

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

## License

[MIT](LICENSE)
