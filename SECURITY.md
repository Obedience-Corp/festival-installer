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

Official marketplace package metadata is signed with an Ed25519 key (id
`obedience-marketplace-2026-01`) whose public half is compiled into the
`festival` binary. By default, `festival` refuses to install or update
anything whose metadata does not verify against that pinned key
(`--allow-unverified` overrides this for unsigned development content, with a
loud warning).

This guarantees that verified package metadata came from the holder of the
marketplace signing key, and has not been altered in transit or at rest since
it was signed.

It does **not** currently cover every file `festival` reads. As of
2026-08-17, `index.json` and `obey-marketplace.json` (the marketplace and
package index files themselves) are unsigned. A party who can tamper with
those files (for example, a compromised marketplace git remote) can still
influence what `festival` offers to install, even though it cannot forge a
signed package's contents. Do not assume index/marketplace-level integrity
beyond what HTTPS/git transport and the marketplace host already provide.
