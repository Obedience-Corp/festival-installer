package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fake obey stands in for the two binaries this step has to tell apart: an
// obey carrying obey main's service family (install, status, uninstall) and one
// carrying the supervised contract from obey #381 (a restart verb, and an
// install that only replaces a live daemon when asked). The legacy fake answers
// `service restart` the way cobra does for a parent with no run function: it
// prints the parent's help and exits 0, so a probe reading exit codes would
// call it supported.
const fakeObeyScript = `#!/bin/sh
dir=$(dirname "$0")
if [ "$1" = "--version" ]; then
  echo "obey version __VERSION__"
  exit 0
fi
if [ "$1" = "version" ]; then
  echo "unknown command" >&2
  exit 1
fi
if [ "$1" = "service" ]; then
  case "$2" in
    --help)
      cat "$dir/service-help.txt"
      exit 0
      ;;
    status)
      if [ ! -f "$dir/status.json" ]; then
        echo "load daemon config: no such file or directory" >&2
        exit 1
      fi
      cat "$dir/status.json"
      exit 0
      ;;
    install)
      if [ "$3" = "--help" ]; then
        cat "$dir/service-install-help.txt"
        exit 0
      fi
      echo install >> "$dir/verbs.log"
      exit 0
      ;;
__RESTART_CASE__
  esac
  cat "$dir/service-help.txt"
  exit 0
fi
exit 1
`

const fakeRestartCase = `    restart)
      echo restart >> "$dir/verbs.log"
      exit 0
      ;;
`

const fakeServiceHelp = `Register the daemon with launchd (macOS) or systemd --user (Linux).

Usage:
  obey service [command]

Available Commands:
  install     Install the daemon service
__RESTART_LINE__  status      Report whether the daemon service is installed, loaded, and answering
  uninstall   Stop the daemon service and remove its unit file

Flags:
  -h, --help   help for service
`

// The prose paragraph is the one obey's own install help carries: it names
// --restart in a sentence whether or not the flag exists, which is why the
// probe reads the flag lines rather than the screen.
const fakeServiceInstallHelp = `Register the daemon with launchd (macOS) or systemd --user (Linux) and start
it when nothing is running yet.

A daemon that is already running is left alone: the unit on disk is refreshed
and the running process keeps serving until something restarts it. Pass
--restart to replace it now.

Usage:
  obey service install [flags]

Flags:
  -h, --help   help for install
__RESTART_FLAG__`

// fakeObey is a stand-in obey staged in an isolated managed bin dir.
type fakeObey struct {
	home string
	dir  string
	path string
}

// writeFakeObey stages a fake obey as the managed binary. supervised chooses
// between obey main's service family and the contract obey #381 adds.
func writeFakeObey(t *testing.T, supervised bool, version string) fakeObey {
	t.Helper()
	home := t.TempDir()
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("make bin dir: %v", err)
	}

	restartCase, restartLine, restartFlag := "", "", ""
	if supervised {
		restartCase = fakeRestartCase
		restartLine = "  restart     Restart the supervised daemon in place\n"
		restartFlag = "      --restart   restart a daemon that is already running so it picks up this binary\n"
	}

	script := strings.NewReplacer(
		"__VERSION__", version,
		"__RESTART_CASE__", restartCase,
	).Replace(fakeObeyScript)
	writeFile(t, filepath.Join(binDir, "obey"), script)
	writeText(t, filepath.Join(binDir, "service-help.txt"), strings.ReplaceAll(fakeServiceHelp, "__RESTART_LINE__", restartLine))
	writeText(t, filepath.Join(binDir, "service-install-help.txt"), strings.ReplaceAll(fakeServiceInstallHelp, "__RESTART_FLAG__", restartFlag))

	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	return fakeObey{home: home, dir: binDir, path: filepath.Join(binDir, "obey")}
}

// reportsDaemon makes `obey service status --json` answer with body. Without a
// call to this the fake fails the status verb, which is the unknown state.
func (f fakeObey) reportsDaemon(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, "status.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write status.json: %v", err)
	}
}

// verbs is every service verb the fake was actually asked to run.
func (f fakeObey) verbs(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.dir, "verbs.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read verbs.log: %v", err)
	}
	return strings.Fields(string(raw))
}

// An obey carrying obey main's service family gets no verb at all. Driving it
// would report a deferred restart obey had already performed, or call a verb
// that command does not have and be told nothing about it.
func TestObeyServiceContract_RefusesObeyMainsServiceFamily(t *testing.T) {
	fake := writeFakeObey(t, false, "0.2.0")

	contract := obeyServiceContractAt(context.Background(), fake.path)
	if contract.Supported {
		t.Fatal("an obey with no restart verb must not be graded as supervised")
	}
	if !strings.Contains(contract.Reason, "obey service restart") {
		t.Fatalf("reason = %q, want it to name the missing verb", contract.Reason)
	}
	if contract.Version != "0.2.0" {
		t.Fatalf("version = %q, want the version the fake reports", contract.Version)
	}

	// The exit code cannot carry this: the fake answers the verb the way cobra
	// does, printing help and succeeding.
	if err := runServiceVerb(context.Background(), fake.path, serviceVerbRestart); err != nil {
		t.Fatalf("the fake must mimic cobra and succeed on an unknown verb, got %v", err)
	}
}

// The other half of the contract: an install that replaces a running daemon on
// its own makes --no-restart a false promise, so the restart verb alone is not
// enough to drive the step. The flag has to be declared, not merely mentioned:
// this fake keeps the prose sentence naming --restart and drops the flag line,
// which is what an obey whose install still restarts unconditionally looks
// like once someone documents the flag it does not have.
func TestObeyServiceContract_RefusesAnInstallThatOnlyMentionsTheFlagInProse(t *testing.T) {
	fake := writeFakeObey(t, true, "0.2.0")
	help := filepath.Join(fake.dir, "service-install-help.txt")
	body, err := os.ReadFile(help)
	if err != nil {
		t.Fatalf("read install help: %v", err)
	}
	var kept []string
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		// Only the declaration goes; the prose that names the flag stays,
		// including the wrapped line that happens to begin with it.
		if trimmed != line && strings.HasPrefix(trimmed, serviceInstallRestartFlag) {
			continue
		}
		kept = append(kept, line)
	}
	stripped := strings.Join(kept, "\n")
	if !strings.Contains(stripped, serviceInstallRestartFlag) {
		t.Fatalf("the prose mention must survive, help = %q", stripped)
	}
	if err := os.WriteFile(help, []byte(stripped), 0o644); err != nil {
		t.Fatalf("write install help: %v", err)
	}

	contract := obeyServiceContractAt(context.Background(), fake.path)
	if contract.Supported {
		t.Fatal("a flag named only in prose is not a flag; this install restarts on its own")
	}
	if !strings.Contains(contract.Reason, serviceInstallRestartFlag) {
		t.Fatalf("reason = %q, want it to name the missing flag", contract.Reason)
	}
}

// helpListsFlag reads the flag lines and only the names on them.
func TestHelpListsFlag_ReadsTheFlagLinesNotTheProse(t *testing.T) {
	withFlag := strings.ReplaceAll(fakeServiceInstallHelp, "__RESTART_FLAG__",
		`      --restart   restart a daemon that is already running`+"\n")
	if !helpListsFlag(withFlag, serviceInstallRestartFlag) {
		t.Fatal("a declared flag must be found")
	}

	proseOnly := strings.ReplaceAll(fakeServiceInstallHelp, "__RESTART_FLAG__", "")
	if !strings.Contains(proseOnly, serviceInstallRestartFlag) {
		t.Fatal("this screen is supposed to mention the flag in prose")
	}
	if helpListsFlag(proseOnly, serviceInstallRestartFlag) {
		t.Fatal("a flag named in the description is not a declared flag")
	}

	// One flag documented in terms of another must not stand in for it.
	sibling := "Flags:\n      --no-restart   the opposite of --restart\n"
	if helpListsFlag(sibling, serviceInstallRestartFlag) {
		t.Fatal("a flag description naming another flag must not pass for it")
	}

	// A shorthand pair and a type placeholder still resolve to the long name.
	shorthand := "Flags:\n  -r, --restart   restart a daemon that is already running\n"
	if !helpListsFlag(shorthand, serviceInstallRestartFlag) {
		t.Fatal("a shorthand pair still declares the long name")
	}
}

func TestObeyServiceContract_AcceptsTheSupervisedVerbSet(t *testing.T) {
	fake := writeFakeObey(t, true, "0.2.0")
	contract := obeyServiceContractAt(context.Background(), fake.path)
	if !contract.Supported {
		t.Fatalf("the supervised verb set must be accepted, refused with %q", contract.Reason)
	}
}

// The floor refuses an obey numbered below the first release that can carry
// the contract, and it grades a prerelease of that release as the release it
// is a candidate for rather than refusing it for its suffix.
func TestObeyServiceContract_FloorRefusesOlderObeyAndAcceptsItsPrerelease(t *testing.T) {
	below := writeFakeObey(t, true, "0.0.9")
	contract := obeyServiceContractAt(context.Background(), below.path)
	if contract.Supported {
		t.Fatal("an obey below the floor must be refused even when it lists the verbs")
	}
	if !strings.Contains(contract.Reason, obeyServiceContractFloor) {
		t.Fatalf("reason = %q, want it to name the floor", contract.Reason)
	}

	prerelease := writeFakeObey(t, true, obeyServiceContractFloor+"-rc.1")
	if got := obeyServiceContractAt(context.Background(), prerelease.path); !got.Supported {
		t.Fatalf("a prerelease of the floor release must clear the floor, refused with %q", got.Reason)
	}
}

// An obey that cannot be asked what it supports is not assumed to support it.
func TestObeyServiceContract_UnreadableObeyIsNotSupported(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "obey")
	if err := os.WriteFile(broken, []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatalf("write broken obey: %v", err)
	}
	contract := obeyServiceContractAt(context.Background(), broken)
	if contract.Supported {
		t.Fatal("a binary that will not answer must not be graded as supervised")
	}
	if !strings.Contains(contract.Reason, "could not be read") {
		t.Fatalf("reason = %q, want it to say the probe could not read the commands", contract.Reason)
	}
}

func TestHelpListsCommand_ReadsOnlyTheIndentedCommandLines(t *testing.T) {
	help := strings.ReplaceAll(fakeServiceHelp, "__RESTART_LINE__", "")
	if helpListsCommand(help, serviceVerbRestart) {
		t.Fatal("a help screen without the verb must not list it")
	}
	if !helpListsCommand(help, serviceVerbInstall) {
		t.Fatal("install is listed in this help screen")
	}
	// The usage line is flush left in a cobra screen only as a heading; a
	// command name that appears unindented is prose, not a command.
	if helpListsCommand("restart the daemon yourself\n", serviceVerbRestart) {
		t.Fatal("an unindented line must not pass for a command")
	}
}

// The probe answers running, not running, or nothing at all, and every failure
// is nothing at all rather than a claim that no daemon was up.
func TestObeyDaemonStateAt_RunningStoppedAndUnknown(t *testing.T) {
	ctx := context.Background()

	running := writeFakeObey(t, true, "0.2.0")
	running.reportsDaemon(t, `{"socket_reachable":true,"daemon_pid":4242}`)
	if got := obeyDaemonStateAt(ctx, running.path); got != daemonStateRunning {
		t.Fatalf("state = %v, want running", got)
	}

	pidOnly := writeFakeObey(t, true, "0.2.0")
	pidOnly.reportsDaemon(t, `{"socket_reachable":false,"daemon_pid":4242}`)
	if got := obeyDaemonStateAt(ctx, pidOnly.path); got != daemonStateRunning {
		t.Fatalf("state = %v, want running for a live pid with an unreachable socket", got)
	}

	stopped := writeFakeObey(t, true, "0.2.0")
	stopped.reportsDaemon(t, `{"installed":true,"loaded":true,"socket_reachable":false}`)
	if got := obeyDaemonStateAt(ctx, stopped.path); got != daemonStateStopped {
		t.Fatalf("state = %v, want stopped", got)
	}

	// obey answered, but not with a report: the old probe read this as "no".
	garbled := writeFakeObey(t, true, "0.2.0")
	garbled.reportsDaemon(t, "Daemon Service\n  Reachable: no\n")
	if got := obeyDaemonStateAt(ctx, garbled.path); got != daemonStateUnknown {
		t.Fatalf("state = %v, want unknown for a report that does not parse", got)
	}

	// A config obey cannot load, or a binary an earlier install left broken.
	refused := writeFakeObey(t, true, "0.2.0")
	if got := obeyDaemonStateAt(ctx, refused.path); got != daemonStateUnknown {
		t.Fatalf("state = %v, want unknown when the status verb fails", got)
	}

	// No managed obey to ask is a real answer: nothing managed is serving.
	absent := writeFakeObey(t, true, "0.2.0")
	if err := os.Remove(absent.path); err != nil {
		t.Fatalf("remove fake obey: %v", err)
	}
	if got := obeyDaemonStateAt(ctx, absent.path); got != daemonStateStopped {
		t.Fatalf("state = %v, want stopped when there is no obey to have started a daemon", got)
	}
}

// writeFile stages an executable stand-in.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil { //nolint:gosec // a test stand-in that has to run
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeText stages one of the help screens the stand-in reads back.
func writeText(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// The contract probe answers a question only a daemon that may be serving
// needs answered, so a stopped daemon never pays for it. The fake here fails
// every call, which would refuse the contract if it were consulted.
func TestObeyServiceSwap_StoppedDaemonDoesNotRunTheContractProbe(t *testing.T) {
	fake := writeFakeObey(t, false, "0.2.0")
	writeFile(t, filepath.Join(fake.dir, "service-help.txt"), "")

	got := obeyServiceSwap(context.Background(), daemonStateStopped, false)
	if got.Unsupported {
		t.Fatalf("a stopped daemon must not be held back by the contract, got %+v", got)
	}
	if !got.Installed || !got.Started {
		t.Fatalf("service result = %+v, want the install verb to have run and started the daemon", got)
	}
}
