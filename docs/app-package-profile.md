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

Signed marketplace release_source can declare `tag_prefix: "camp-app-v"` for the
binary-only Obedience-Corp/obey-releases repository. Only that producer's matching
stable/channel tags participate in resolution. Without a prefix, existing plain
v-prefixed semantic tags remain compatible. Declare darwin/amd64 OS/arch token
maps when producer filenames use those tokens instead of macOS/x86_64. Prefix,
source URLs and checksum anchor are covered by the existing marketplace signature.
Changing the prefix without a new valid signature is rejected.

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
