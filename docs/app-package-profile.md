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

An existing destination must still match the owning receipt's hash before it is
replaced. Foreign files/symlinks, user-edited runtime assets and standalone installs
retain ownership and receive an explicit next action. A compatible standalone
Camp App can coexist without changing its receipt/configuration. Uninstall still
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
readback, repeat skip, upgrade, missing-file repair, newer-version preservation,
corrupt download rollback, foreign replacement preservation and namespace tampering.
The fixture signing key is generated for the test; it is not official production
marketplace acceptance. Separate PowerShell/native-platform lanes remain separate
claims and are not implied by these Linux tests.
