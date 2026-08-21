# Security Policy

`festival` downloads package metadata and binary artifacts from marketplaces
and executes them on your machine. Security reports are taken seriously.

## Reporting a vulnerability

Email **security@obediencecorp.com**. Please do not open a public issue for a
suspected vulnerability.

Include what you found, the affected version or commit, and, if you have
one, a minimal reproduction. You can expect an acknowledgment within 5
business days and a decision on scope and fix timeline within 14 days of
that acknowledgment.

## Scope

In scope:

- Bypassing signature verification for marketplace package metadata.
- Path traversal or symlink escape during archive extraction into
  `FESTIVAL_HOME`.
- Install transaction or journal corruption that results in the wrong
  binary, or a partially-written binary, being placed on `PATH`.
- Marketplace metadata parsing bugs that could be used to smuggle unexpected
  file writes or command execution through a crafted manifest.

Out of scope: vulnerabilities in `camp`, `fest`, or plugins installed
*through* `festival`. Report those to the repository that owns the affected
tool.

## What the trust root guarantees, and what it does not

### What is signed

`obey-marketplace.json`, `index.json`, and every `packages/**/obey-package.json`
are each signed with a detached Ed25519 signature over the exact canonical
bytes stored on disk, verified against the key pinned in
`internal/verify/trust.go` (id `obedience-marketplace-2026-01`).

`index.json` is signed and independently verifiable
(`festival-metadata verify --pinned --kind index`), but `festival` itself
does not read `index.json` on any path today; it is signed for the
publisher's own integrity guarantee and for future consumers, not because
the installer enforces it. Do not assume signing a document means `festival`
checks it: the next two paragraphs describe only `obey-marketplace.json` and
the package manifests, which is everything the installer actually reads.

### Where it is enforced

Every read path checks the signature of `obey-marketplace.json` and, when
installing a package, its manifest: seed, `marketplace add`,
`marketplace list`, `marketplace refresh`, `browse`, `install`, and plugin
update. The official marketplace refuses unsigned or non-verifying content by
default; a present but invalid signature is never overridable, on the
official source or any other. A user-added third-party marketplace has no key
infrastructure behind it, so unsigned content there is allowed with a loud
warning instead of refused (`--allow-unverified` on `marketplace add`,
`marketplace refresh`, and `browse` makes that warning path explicit).

### What the chain buys for plugins

A git-release plugin installed from a verified marketplace source is
verified by chain: pinned key, signed marketplace document, the
`checksums_url` it declares, sha256, then the asset itself. The plugin never
carries its own signature; it inherits trust from the marketplace document
that names it.

### What is still not covered

- **Freshness.** A signature proves who produced a document and that it has
  not changed since, not which version you were served. A party that can
  serve an older, validly signed commit of the marketplace can roll a fresh
  install back to a previous version, and a fresh install has no prior state
  to compare against. There is no signed timestamp and no maximum-age policy
  on any of the three documents today. See `festival doctor`'s
  `marketplace_trust` check for the current verification state of every
  registered source; it does not detect this, because a rolled-back document
  still verifies.
- **Key rotation.** Exactly one key is pinned
  (`internal/verify/trust.go`). There is no second key and no rotation
  procedure, so a key compromise requires a new hub release.
- **Third-party marketplaces.** Unsigned by design, warned about, not
  refused.
- **The host serving `checksums_url` and artifact URLs.** The sha256
  comparison assumes that host is honest about what it serves under a given
  URL, the same assumption the signed package manifest path already makes.
