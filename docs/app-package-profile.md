# Selected app package integration

The App owns its required/default/optional profile. The marketplace remains the
broader signed discovery catalog. Installer operations act on explicit targets;
they do not update every installed plugin.

Use an exact marketplace identity to avoid selecting another publisher:

```sh
festival status --plugin obedience-corp/camp-app --json
festival install obedience-corp/camp-app --json
festival update obedience-corp/camp-app --json
festival uninstall obedience-corp/camp-app --json
```

`status --plugin` is repeatable and read-only. Core status remains compatible.
Core tool entries also expose an optional `bundle`: the suite release reported
by that resolved binary during discovery. Camp and Fest component versions use
their own release numbers; a Camp `version` of `0.9.1` can belong to suite
`bundle` `v0.3.9`. Compare a suite minimum with its suite identity, not with a
component version. Mixed installations can report different bundles per tool.
The field is omitted when discovery has no metadata for the resolved path;
it never borrows another copy's bundle or infers one from the Installer's own
version. `receipt_version` still describes the package's stored receipt, which
may differ from an externally resolved binary. Consumers must consider the
resolved path and origin when interpreting that receipt.
Selected plugin facts include live version, receipt version, path and origin.
Camp App additionally reports its versioned control protocol and capabilities from
`version --json`; an unsuccessful/malformed payload is an error, not a usable
version. Unrelated plugins are not probed.

Update uses the receipt's source/channel, verifies signed metadata and final
checksums, and reuses the existing adapter and transaction journal. Results are
`current`, `upgraded`, `repaired`, `absent` or `unmanaged`. Missing or wrong-mode
owned files can be repaired at the same version. Current/newer intact installs
are kept; implicit downgrades are refused, including inside the activation lock.

With `--managed`, updating a selected plugin without an Installer receipt creates
a separate verified copy in the Installer's bin directory. This lets App setup
repair an outdated standalone Camp App without requiring a terminal command. The
selected marketplace is refreshed and verified first so a cached release does not
keep the App below its minimum. Subsequent managed status/resolve calls select the
new copy; the original binary, symlink, receipt and configuration remain intact.
Without `--managed`, an unowned plugin still returns `unmanaged` with guidance.

An existing destination must still match the owning receipt's hash before it is
replaced, even with `--managed`. Foreign files/symlinks and user-edited runtime
assets retain ownership and receive an explicit next action. Uninstall still
removes only recorded identities. No `--force` flag grants plugins blanket foreign
file overwrite permission.

## Shared public binary repositories

The shared official catalog must remain readable by released clients. Installer
0.3.6 rejects unknown release_source fields for the whole catalog, so Camp App
uses the existing signed static package-manifest format with exact product-tagged
public URLs and hashes. Marketplace's prepare-camp-app command generates it only
from actual public release bytes, preserving all existing discovery entries.
Windows artifact.binaries can explicitly declare camp-app.exe while the install
entry keeps the portable command name camp-app; custom source members are kept.

The new tag_prefix extension is available only to catalogs whose consumers support
it. Do not add it to the shared legacy catalog. In such a compatible catalog it
filters a literal producer tag namespace; existing plain version tags remain
compatible when absent. Prefix/source/checksum changes remain covered by the
marketplace signature.

Apple Developer ID/notarization is optional post-launch. This does not relax
marketplace authenticity, artifact checksums or the App updater signature.

## Verification

Run `just plugin-profile container`. The full default vet/race suite and targeted
filesystem profile tests run as an unprivileged disposable container user. The
signed local-catalog lifecycle exercises actual artifact downloads, receipt/file
readback, standalone-to-managed repair, stale catalog refresh, repeat skip, upgrade,
missing-file repair, newer-version preservation, corrupt download rollback/retry,
cancelled repair, foreign replacement preservation and namespace tampering.
The fixture signing key is generated for the test; it is not official production
marketplace acceptance. Separate PowerShell/native-platform lanes remain separate
claims and are not implied by these Linux tests.

## Festival App package

**Identity and catalog entry.** Package id `obedience-corp/festival-app`. The catalog entry is exactly
`{"id","display_name":"Festival App","class":"product","summary","channels":["stable"],"manifest_path":"packages/obedience-corp/festival-app/obey-package.json"}`:
no `release_source`, no `targets`, no new catalog fields. The catalog schema is closed and every released
installer reads the whole catalog, including v0.3.12 bundled in Festival App 0.1.7; one unknown field breaks
every operation on the official source (the 0.3.6 note above is the same failure). The marketplace's
`festival-app` preparer writes this entry from published `app-vX.Y.Z` releases only.

**Manifest kinds.** Artifact kinds `app-bundle` (macOS `.app.tar.gz`), `nsis` (Windows `_x64-setup.exe`),
`deb`, `appimage`; optional `updater_signature` holding the Tauri `.sig` file contents as published; install
entry kind `desktop_app` with `bundle_id` (`com.obediencecorp.festival-app`) and `product_name` (`Festival App`).
Artifacts are chosen by kind (`SelectAppArtifact`), so the deb and the AppImage can both target linux/amd64.
An App install refuses an artifact without `updater_signature` (`E_APP_SIGNATURE`).

**Compatibility rule.** New kinds and fields live only in the App's manifest. Manifests load per package, so an
installer that predates App support never loads it and fails only for the App (`E_INSTALL_TARGET`). The suite
manifest must never use these kinds, and `SelectArtifact` for every other package is unchanged.

**Receipt.** One receipt row for the App's main executable with an empty hash, and the metadata keys
`app.format`, `app.location`, `app.bundle_id`, `app.artifact_sha256`, `app.adopted`, `manifest_url`, in the
shared receipts database without a migration, so the App's bundled installer keeps reading the same state.
Released installer v0.3.12 reports the App as `external`, skips it in `update --all`, and its TUI uninstall
refuses it (`E_UNINSTALL_PATH`) without removing anything.

**Updater key and rotation.** festival pins the App's updater public key from festival-app
`src-tauri/tauri.conf.json` (key id `A0880EE086F129E5`) and checks the minisign signature over each artifact.
Rotating that key needs an installer release: publish an installer that pins the new key before the first App
release signed with it. Installers that only know the old key fail that release with `E_APP_SIGNATURE` and
leave the installed App in place until festival is updated.

**Verification.** `just testing app-container-proof` runs the resolver, download, journal and swap, receipt,
status, update, uninstall and CLI tests in a Linux container as an unprivileged user; it proves the logic and the
crash recovery, not a real OS installer. `just testing windows-install-container-proof` runs the Windows
container tests, including the NSIS flow against a fake installer. The `app_native` tests run only on fresh
hosted macOS, Windows and Ubuntu runners and place the real 0.1.7 artifacts: the macOS code signature and
quarantine state, the real NSIS installer and uninstaller, the real deb through pkexec, and the real AppImage.

