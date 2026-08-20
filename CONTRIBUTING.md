# Contributing

Contributions are welcome. By contributing you agree that your contribution is
licensed under the [Apache License 2.0](LICENSE), the same license as the
project.

## Developer Certificate of Origin

Every commit must be signed off:

```bash
git commit -s
```

The sign-off certifies the [Developer Certificate of Origin 1.1](https://developercertificate.org/):
that you wrote the change, or otherwise have the right to submit it under the
project's license. Pull requests with unsigned commits will be asked to rebase
with sign-offs before merge.

## Practical notes

- Run the full gate before opening a PR: `just ci`. It runs formatting, vet,
  golangci-lint, the test suite, all four cross-platform release builds, and the
  static-linking check on the Linux artifact.
- The installer places and replaces binaries on a user's machine. Any change to
  `internal/installer`, `internal/verify` or `internal/state` needs a test that
  covers the failure path, not only the happy path.
- Match the surrounding code's conventions; see the README for project layout.
