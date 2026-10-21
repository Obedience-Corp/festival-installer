# Automatic setup and repair

`festival setup integrations` is the common app/CLI setup operation. It caches
camp/fest/Obey-generated helpers, adds one owned startup block, verifies a fresh
shell, and installs current user provider hooks. Shell startup uses local files
and performs no installer network check. `festival setup` without a subcommand
still creates the starter camp.

The JSON envelope's `data.schema` is `festival-integrations/v1`. Its fields are
`shell`, `ready`, `changed`, `new_terminal`, `disabled`, and `checks`. Each check
has an `id`, `status`, `message`, optional `path`, and `repairable`. In JSON mode, exit success
means a report was produced; callers must inspect readiness. Without `--json`,
not-ready reports exit nonzero, including root refusal and a persisted opt-out.
An explicit `--disable` exits successfully when removal and persistence succeed. `upgrade_required`
means the resolved tool cannot generate its integration, so the app can update
it and retry. Read-only `--check` does not write installer state, startup files,
or hooks, but fresh-shell verification executes the user's startup scripts.

## Customization and removal

Bash setup covers `.bashrc` and the first existing login profile. Zsh honors
`ZDOTDIR`, Fish honors `XDG_CONFIG_HOME`, and PowerShell uses its resolved
profile. Detection retains the absolute login-shell executable, including a
Homebrew shell outside the GUI's PATH. On Windows, automatic setup configures
and probes both installed PowerShell editions independently, even when a
Git Bash launch provides a POSIX `SHELL`; PowerShell 7 and Windows PowerShell 5
use different profiles. Missing shell executables are
reported separately from startup/helper failures. An explicit `--shell`
(name or absolute executable path) overrides detection. Unsupported
shells and malformed/duplicate Festival blocks are reported for repair.
Existing aliases/functions are preserved and may prevent automatic wrapping;
explicit `obey wrap <provider>` remains available. Scripts and pipelines keep
their normal behavior. PowerShell's owned block is limited to console prompt
sessions: script, command, encoded-command and ordinary piped invocations do
not load helpers or change PATH. Other PowerShell hosts/custom launch modes
are left alone and may require explicit integration. Automatic setup intentionally
does not change the Windows user Path registry: the managed bin directory is
available in configured interactive PowerShell prompts, but is not added to
`cmd.exe`, Explorer launches, or scripts. Use explicit executable paths there,
or choose the separate advanced PATH setup if process-wide discovery is wanted.
Zsh keeps its usual completion security prompt in ordinary terminals; only the
bounded readiness child skips insecure completion directories without prompting.

Changes preserve unrelated bytes and permissions, follow rc symlinks, and save
backups under `$FESTIVAL_HOME/backups/integration` (default home:
`~/.obey/installer`). Setup uses the installer lock and rejects concurrent file
changes before replacement. Backups are written atomically and checked on
retry, including recovery from an incomplete earlier backup. Repeated setup is
idempotent. Root/sudo invocations skip all integration and provider-hook work
and return a repairable not-ready report; run setup as the normal user.

`--disable` removes tracked owned startup blocks and persists the preference.
Provider-hook repair is independent: a hook warning does not fail a completed
opt-out.
`--enable` restores automatic configuration. `install --no-shell-setup` records
the same opt-out. Provider hooks remain useful for observation and explicit
wrapped launches. Uninstall refreshes the helper for remaining components and
removes owned blocks when none remain. Receipt-owned Obey uninstall first removes
its user-scoped provider hooks, then its service, while the binary still exists.
Root/sudo Obey uninstall refuses before cleanup or binary deletion; rerun as the
normal user so the hooks and binary are removed under the same identity.
A cleanup failure is reported with a retry path; binary removal still follows
the existing uninstall warning contract. Installs without receipts are left alone. Cached helper files, preferences, and
backups are retained; unrelated configuration is never removed.

## App-managed toolchains and prerequisites

The global `--managed` flag selects a separate installer-managed toolchain for
status, resolve, install, update, and integration setup. The app passes it on
every installer invocation. When a package-owned copy is incompatible, a signed
managed copy is installed in the user installer bin directory; no Homebrew,
AUR, or other package files are overwritten. Reported tool paths show the copy
in use. Generated shell helpers put those tool directories first. CLI calls
without this explicit mode retain existing package ownership rules.

`festival setup prerequisites --json` installs missing Git using the OS:
macOS developer tools, Windows Git.Git through winget, or a supported Linux
package manager through desktop pkexec authorization. Festival never captures
an administrator password. Linux requires pkexec and apt-get, dnf, pacman, or
zypper; Windows requires winget. Offline, denied authorization, or unavailable
system installers produce a retryable explanation. macOS installation can be
pending while the system dialog runs. Status checks query `xcode-select -p`
before executing Apple's Git launcher, so missing developer tools do not raise
an installation dialog from a read-only status check. The JSON command returns
a report even when Git is unavailable; callers inspect `ready` and `pending`.
The human command exits nonzero until Git is ready, including while the macOS
developer-tools installer is pending. Some Windows environments need the app
reopened after Git installation to inherit the new PATH.

## Releases

Ship the installer API before an app that requires it. App packaging verifies
the staged installer's schema and managed-mode support in addition to release
checksums. Obey must contain `wrap shell-init` and compatible provider hooks.
Obey is proprietary and is installed from distributed artifacts, never by
requiring customer access to its private source repository.

The desktop app can set `FESTIVAL_APP_TOOL_BIN` to its existing absolute private
launcher directory. Helpers keep that directory first after adding tool parents,
so a stale executable beside another selected tool cannot shadow the app's exact
resolved executables. Empty, relative, or missing directories are ignored.
Ordinary terminals without this launch environment retain normal managed PATH
ordering. The app owns the directory and keeps it alive for its terminal sessions.
