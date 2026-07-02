# obey-installer

Installer engine and plugin manager for `fest`, `camp`, and future Obey tools.

## Status

Private incubation repo. Not yet ready for external use. The mature engine will
merge into the public `festival` binary once foundations stabilize. Until then,
`projects/festival` remains the public bootstrap surface.

### What works today

The `install`, `update`, `uninstall`, `browse`, `marketplace`, and `doctor`
subcommands are wired and functional (`cmd/obey-installer/main.go`, backed by
`internal/cli`); `list` is still a stub. `version`, `help`, and `completion`
work as expected.

> **Security status (important):** package metadata is currently consumed
> **without signature verification.** The `internal/verify` apparatus (RFC 8785
> canonical JSON + ed25519) exists and is tested, but the live install path in
> `internal/source` does not yet route through it, and no trust root is pinned.
> Wiring mandatory verification is tracked by festival
> `obey-installer-security-OI0007` (finding VER-01). Until it lands, treat
> installs as trusting the source repository, and only add sources you control.
> Transport is HTTPS-only, git invocation is hardened against argument and
> protocol injection, and archive extraction is bounded against decompression
> bombs (findings VER-03, VER-04, EXT-01).

### What's implemented under the hood

The foundation packages are complete and tested:

| Package | Surface |
|---|---|
| `internal/state` | SQLite handle with WAL + embedded migration runner (`OpenDB`, `Close`, `Conn`) |
| `internal/state/receipts` | Receipt CRUD (`Write`, `Get`, `List`, `Delete`) with FK-cascaded files and metadata |
| `internal/state/lock` | POSIX `fcntl`-backed cross-process install lock (`FileLock`) |
| `internal/verify` | RFC 8785 canonical JSON `Marshal` and ed25519 `Verify` with pluggable `KeyStore` (built and tested; not yet wired into the live install path, see VER-01) |
| `internal/metadata` | Schema-validated parsers for `source.json` / `index.json` / package manifest, plus `ParseVerified*` helpers that canonicalize + verify + parse |
| `internal/artifacts` | HTTPS-enforced download, bounded tar.gz extraction, atomic move |
| `internal/gitsafe` | Git remote scheme validation and injection-hardened invocation flags |

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
