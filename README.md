# obey-installer

Installer engine and plugin manager for `fest`, `camp`, and future Obey tools.

## Status

Private incubation repo. Not yet ready for external use. The mature engine will
merge into the public `festival` binary once foundations stabilize. Until then,
`projects/festival` remains the public bootstrap surface.

### What works today

The CLI is a scaffold. Only `version`, `help`, and `completion` do anything real:

```bash
$ obey-installer version
0.0.0-dev

$ obey-installer install fest
install: not implemented
```

The user-facing subcommands (`install`, `browse`, `list`, `update`, `uninstall`,
`marketplace`, `doctor`) all exit with `not implemented`. They will be wired up
as later festivals land Steps 3–10 of the implementation plan.

### What's implemented under the hood

The foundation packages (festival `obey-installer-foundations-OI0003`, Steps 1–2
of the design plan) are complete and tested:

| Package | Surface |
|---|---|
| `internal/state` | SQLite handle with WAL + embedded migration runner (`OpenDB`, `Close`, `Conn`) |
| `internal/state/receipts` | Receipt CRUD (`Write`, `Get`, `List`, `Delete`) with FK-cascaded files and metadata |
| `internal/state/lock` | POSIX `fcntl`-backed cross-process install lock (`FileLock`) |
| `internal/verify` | RFC 8785 canonical JSON `Marshal` and ed25519 `Verify` with pluggable `KeyStore` |
| `internal/metadata` | Schema-validated parsers for `source.json` / `index.json` / package manifest, plus `ParseVerified*` helpers that canonicalize + verify + parse |

These are library primitives; nothing in `cmd/` calls them yet.

## Design Reference

See `workflow/design/festival-plugin-marketplace/` in the `obey-campaign`
workspace for the architecture, security model, and rollout plan. The
implementation roadmap lives at `10-implementation-plan.md` (Steps 1–2 shipped,
Steps 3–10 ahead).

## Development

All development commands flow through `just`:

```bash
just                  # list all recipes
just build            # build bin/obey-installer
just run version      # go run ./cmd/obey-installer version
just check            # fmt + vet + lint + test (pre-commit gate)
just ci               # check + cross-platform release builds
```

### Testing

```bash
just test                  # go test ./...
just testing race          # with race detector
just testing coverage      # with coverage summary
just testing coverage-html # writes coverage.html
just testing bench         # run benchmarks
just testing verbose       # verbose output
```

### Cross-platform builds

```bash
just release all           # all four targets
just release linux         # linux/amd64
just release linux-arm64   # linux/arm64
just release darwin        # darwin/amd64
just release darwin-arm64  # darwin/arm64
```

### Linter

`golangci-lint` is required for `just lint` and `just check`. Install with:

```bash
just tools install-golangci-lint
```

The lint recipe will fail with this hint if the binary is missing.

### Module hygiene

```bash
just tidy   # go mod tidy && go mod download
just fmt    # go fmt ./...
just vet    # go vet ./...
just clean  # remove bin/ and coverage artifacts
```

## License

FSL-1.1-ALv2. See LICENSE.
