# festival

`festival` installs, updates, and launches the Festival CLI suite (`camp` and
`fest`) and their plugins, pulling packages from a signed marketplace. Bare
`festival` opens a fire-themed TUI; the same binary answers to CLI
subcommands for scripts and agents.

> _All your work, where you can find it._

## Demo

Recorded against a real `./bin/festival` binary (VHS + PTY).

**Home: boot splash, ambient flame, multi-activity booths, menu → doctor**

<p align="center">
  <img src="docs/demos/festival-home.gif" alt="festival TUI: boot splash, fire ambient, multi-activity booths, and home menu" width="900">
</p>

**Tour: install channel picker, shell/PATH, installed list**

<p align="center">
  <img src="docs/demos/festival-tour.gif" alt="festival TUI tour: install channel picker, shell/PATH, and installed packages" width="900">
</p>

**Launchpad: open camp as a child tool, quit back to the hub**

<p align="center">
  <img src="docs/demos/festival-launchpad.gif" alt="festival TUI launchpad: opening camp as a child tool and returning to the hub on quit" width="900">
</p>

Reproduce locally:

```bash
just build
just vhs all   # requires vhs, ttyd, ffmpeg
```

## Install

**npm / pnpm / bun:**

```bash
npm install -g @obedience-corp/festival
```

**macOS:**

```bash
brew install --cask Obedience-Corp/tap/festival
```

**Arch Linux:**

```bash
yay -S festival-bin
```

**Shell script (macOS and Linux):**

```bash
curl -fsSL https://raw.githubusercontent.com/Obedience-Corp/festival/main/install.sh | bash
```

Each of these installs the whole suite (`camp`, `fest`, and `festival`), not
just this binary; they mirror the [`festival` distribution repo's
README](https://github.com/Obedience-Corp/festival#install). Suite releases
that predate the hub install `camp` and `fest` only; the first release that
ships all three is the one that adds `festival` to every method above.

Festival Installer lives at `github.com/Obedience-Corp/festival-installer`;
its public command name is **`festival`**. CI tests the supported Linux and
macOS targets on amd64 and arm64. Tags matching `v*` publish portable
binaries and checksums through GitHub Releases. Until the first tag is
published, [build it from source](#build-from-source).

## Trust model

The marketplace document (`obey-marketplace.json`), the package index
(`index.json`), and every package manifest are signed with an Ed25519 key, id
`obedience-marketplace-2026-01`, whose public half is compiled into the
binary (`internal/verify/trust.go`). By default `festival` refuses to install
or update anything whose metadata it cannot verify against that pinned key.
To proceed anyway with unsigned development content, pass
`--allow-unverified` on the CLI (it prints a loud warning to stderr) or
accept the equivalent explicit consent prompt the TUI shows when it refuses
unsigned content. Artifact downloads are HTTPS-only, git remotes are
restricted to HTTPS, SSH, and local paths, and archive extraction is bounded.
Run `festival doctor` to check the trust state of every registered source in
one command; see `SECURITY.md` for what signing does and does not cover.

## Quick start (humans)

```bash
just build          # → bin/festival
./bin/festival      # opens the TUI on a terminal
```

The TUI home screen lets you:

- Install / update the Festival suite (camp + fest)
- List installed packages
- Browse the catalog and install plugins
- Uninstall receipt-owned packages
- Manage marketplaces
- Run doctor and PATH / shell-init guidance
- **Launchpad**: open camp/fest tools (`camp wi`, `fest watch`, …) as real
  subprocesses; quit the tool to return to the hub without relaunching `festival`
  (suspend → child → resume)

**Animations:** boot splash, ambient multi-activity “booths”, progress flame, and
a short success burst. Disable ambient motion with:

```bash
export FESTIVAL_REDUCED_MOTION=1
```

## CLI (scripts / agents)

```bash
festival install festival --channel stable
festival update festival
festival list
festival list --json
festival browse --product fest --kind plugin
festival marketplace list
festival doctor
festival shell-init zsh
festival which camp --show-all
festival version
```

JSON envelopes use schema version `festival/v1alpha1`.

### Install targets

| Target                       | Effect                                                           |
| ---------------------------- | ---------------------------------------------------------------- |
| `festival`, `camp`, `fest`   | Install the suite bundle `obedience-corp/festival` (camp + fest) |
| `camp-<name>`, `fest-<name>` | Install a plugin from registered marketplaces                    |

## Home directory

| Env             | Role                                       |
| --------------- | ------------------------------------------ |
| `FESTIVAL_HOME` | Absolute path override for manager state   |

Default: `~/.obey/installer` with `bin/`, `state.db`, marketplace clones, receipts.

## Build from source

Requires Go 1.25.6 or newer (see `go.mod`).

```bash
just                  # list recipes
just build            # bin/festival
just run version
just check            # fmt + vet + lint + test
just test
just release all      # cross-platform festival-{os}-{arch}
just ci                # full gate: check + all four release builds + static-link check
```

### Layout

```text
cmd/festival/          # binary entry (bare → TUI, subcommands → CLI)
internal/app/          # service layer shared by CLI and TUI
internal/cli/          # cobra wrappers + human/JSON rendering
internal/tui/          # bubbletea manager (theme, anim, screens)
```

## What works under the hood

| Package                   | Surface                                                   |
| ------------------------- | --------------------------------------------------------- |
| `internal/state`          | SQLite + WAL, home, config                                |
| `internal/state/receipts` | Install receipts                                          |
| `internal/state/lock`     | Cross-process install lock                                |
| `internal/verify`         | Canonical JSON + Ed25519 verification and pinned trust    |
| `internal/artifacts`      | HTTPS download, bounded tar.gz, atomic move               |
| `internal/source`         | Marketplace clone cache + package index                   |

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.
