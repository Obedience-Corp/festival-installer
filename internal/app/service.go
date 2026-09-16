package app

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// serviceTimeout bounds the supervisor verbs. launchctl bootstrap, its removal
// counterpart and a status read all answer in well under a second on a healthy
// machine; the budget exists so a wedged supervisor cannot hold the app's
// launch.
const serviceTimeout = 30 * time.Second

// serviceRestartTimeout bounds the restart verb, which is not a supervisor call
// but a supervised process swap, so this cap has to sit above obey's own
// bounded sequence rather than beside it.
//
// `obey service restart` boots the launchd job out, waits for launchd to
// release the label before it bootstraps the rewritten unit (the unit's own
// ExitTimeOut, 30s, plus a 5s margin), bootstraps, then waits for the
// replacement process to exist and answer a Ping (30s). The outgoing daemon's
// exit wait is subsumed by the label wait, because launchd holds the label
// until that process is gone. So 65s of bounded waiting, and the launchctl
// calls around it are the only unbounded part, sub-second on a healthy machine.
// The numbers are obey's, from its shutdown budget package: a 10s drain, a 5s
// graceful gRPC stop and a 5s teardown make a 20s shutdown, the unit adds a 10s
// supervisor margin on top of it, and the replacement gets 30s to boot.
//
// The cap is 120s rather than a tighter fit over those 65s. What it protects
// against is a wedged supervisor holding the install; what it costs when it
// fires early is worse than waiting, because the installer then kills obey in
// the middle of a swap it was still performing and reports a failure for a
// restart that was inside obey's own contract. At 120s it fires only once obey
// has blown that contract, and by then obey's own error is the one the user
// should be reading. It also survives a modest growth in obey's budgets, which
// a 25s margin would not.
const serviceRestartTimeout = 120 * time.Second

// codeServiceTimeout marks a verb the installer cut short rather than one the
// supervisor refused. The two need different words: a refusal means the daemon
// is not coming up, a timeout means nobody knows yet.
const codeServiceTimeout = "E_SERVICE_TIMEOUT"

// codeServiceContract marks a contract probe that could not read what the
// staged obey can do. It is never an install failure: it means the service
// step is skipped, not that the binaries did not land.
const codeServiceContract = "E_SERVICE_CONTRACT"

// serviceWaitDelay bounds the wait for output pipes after the budget kills the
// command. Killing obey does not kill the launchctl or systemctl child it
// spawned, and that grandchild holds the pipe open, so without this the budget
// bounds nothing: the call returns whenever the supervisor happens to finish.
const serviceWaitDelay = 2 * time.Second

// Service verbs, matching the commands obey registers. They are constants
// because the installer calls them by name and reads only the exit code, so a
// rename in obey is a one-line change here.
const (
	serviceVerbInstall   = "install"
	serviceVerbRestart   = "restart"
	serviceVerbUninstall = "uninstall"
	serviceVerbStatus    = "status"
)

// obeyServiceContractFloor is the lowest obey version whose service family can
// carry the contract these steps drive: `obey service restart` exists, and
// `obey service install` refreshes the unit without replacing a daemon that is
// already serving. Both arrive in obey #381.
//
// obey publishes no tags and its binary reports 0.1.0, so the first release
// that can contain #381 is 0.1.0. The floor therefore refuses only an obey
// numbered below its current version, and the verb probe below is what
// separates a 0.1.0 built before #381 from one built after. Raise the floor to
// the release that first ships #381 once obey tags one, and the probe becomes
// the second opinion rather than the only one.
const obeyServiceContractFloor = "0.1.0"

// serviceInstallRestartFlag is the flag obey #381 adds to `service install`.
// Its absence is how an install that replaces a running daemon on its own is
// recognised: the flag exists precisely because the default stopped doing that.
const serviceInstallRestartFlag = "--restart"

// verbTimeout is the budget for one verb.
func verbTimeout(verb string) time.Duration {
	if verb == serviceVerbRestart {
		return serviceRestartTimeout
	}
	return serviceTimeout
}

// ServiceResult reports what the installer did with the obey user service.
// It is advisory: a false Installed or Restarted with a non-empty Error means
// the binaries landed and the supervisor did not take them.
type ServiceResult struct {
	// Installed is true when obey service install succeeded on this run.
	Installed bool `json:"installed"`
	// Restarted is true when obey service restart succeeded on this run.
	Restarted bool `json:"restarted"`
	// Deferred is true when a restart was due and the caller passed --no-restart.
	Deferred bool `json:"deferred"`
	// Error is the service command's failure, empty when nothing failed.
	Error string `json:"error,omitempty"`
	// TimedOut is true when the installer cut the verb short at its budget.
	// The supervisor may still be finishing it, which is why this is reported
	// apart from a refusal.
	TimedOut bool `json:"timed_out,omitempty"`
	// Started is true when the service step brought up a daemon that was not
	// running before, rather than restarting one that was. Set on install and
	// on update alike, because both hand a unit to the supervisor for a
	// machine with nothing serving; only the human sentence is update-only.
	// It is never set from a probe that could not tell.
	Started bool `json:"started,omitempty"`
	// DaemonStateUnknown is true when the probe that runs before the binaries
	// change could not establish whether a daemon was serving. The step then
	// treats the daemon as one that may be running: it restarts, or with
	// --no-restart defers and says the state could not be read.
	DaemonStateUnknown bool `json:"daemon_state_unknown,omitempty"`
	// Unsupported is true when the staged obey predates the supervised daemon
	// contract and the daemon was serving or could not be read, so no service
	// verb ran. An obey below the contract still registers its unit on a
	// daemon the probe definitely found stopped, and that is an ordinary
	// install rather than this.
	Unsupported bool `json:"unsupported,omitempty"`
	// ContractReason names what was missing when Unsupported is true.
	ContractReason string `json:"contract_reason,omitempty"`
}

// daemonState is what the probe could establish about the daemon serving right
// now. The three values are kept apart because a probe that could not answer
// must not be reported as an answer: read as "not running" it makes the
// installer claim it started a daemon, read as "running" it claims a pending
// restart, and at most one of those is true.
type daemonState int

const (
	// daemonStateUnknown means the probe failed: the binary would not run, the
	// budget ran out, obey could not load its config, or the report did not
	// parse. Nothing is claimed about the daemon on this answer.
	daemonStateUnknown daemonState = iota
	// daemonStateStopped means obey answered and reported no daemon, or there
	// is no managed obey on disk that could have started one.
	daemonStateStopped
	// daemonStateRunning means obey answered and reported a daemon serving.
	daemonStateRunning
)

// serviceStep runs one obey service verb with the freshly staged binary.
// Errors are reported, never returned: a daemon that did not register is a
// degraded install, not a failed one.
func serviceStep(ctx context.Context, verb string) ServiceResult {
	var res ServiceResult
	obeyPath, err := obeyServicePath(ctx)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if err := runServiceVerb(ctx, obeyPath, verb); err != nil {
		res.Error = err.Error()
		res.TimedOut = errpkg.Code(err) == codeServiceTimeout
		return res
	}
	switch verb {
	case serviceVerbInstall:
		res.Installed = true
	case serviceVerbRestart:
		res.Restarted = true
	}
	return res
}

// runServiceVerb runs one obey service verb with the given obey binary. The
// returned error is the command's own failure; callers turn it into a warning
// rather than failing an install.
func runServiceVerb(ctx context.Context, obeyPath, verb string) error {
	return runServiceVerbWithin(ctx, obeyPath, verb, verbTimeout(verb))
}

// runServiceVerbWithin is runServiceVerb with an explicit budget, so the
// timeout path can be exercised without waiting out a real one.
func runServiceVerbWithin(ctx context.Context, obeyPath, verb string, budget time.Duration) error {
	runCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	cmd := exec.CommandContext(runCtx, obeyPath, "service", verb) //nolint:gosec // obeyPath is the managed bin dir, verb is a constant
	cmd.Stdin = nil
	cmd.WaitDelay = serviceWaitDelay
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if ctx.Err() == nil && runCtx.Err() != nil {
		return errpkg.Wrap(codeServiceTimeout, runCtx.Err(),
			"obey service "+verb+" timed out after "+budget.String())
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return errpkg.Wrap("E_SERVICE_VERB", err, "obey service "+verb+": "+detail)
}

// serviceReport is the subset of `obey service status --json` the installer
// reads. The daemon is "running" when it answers its socket or its pid file
// names a live process, which is the same pair obey itself grades on.
type serviceReport struct {
	SocketReachable bool `json:"socket_reachable"`
	DaemonPID       int  `json:"daemon_pid"`
}

// obeyDaemonState asks the obey binary that is on disk right now whether a
// daemon is already up. It is the question install has to answer before it
// replaces that binary: a daemon that was running keeps running the old image
// until something restarts it.
func obeyDaemonState(ctx context.Context) daemonState {
	obeyPath, err := obeyServicePath(ctx)
	if err != nil {
		return daemonStateUnknown
	}
	return obeyDaemonStateAt(ctx, obeyPath)
}

// obeyDaemonStateAt is obeyDaemonState asked of an explicit binary.
//
// Only obey's own answer, or the absence of any obey to ask, decides between
// running and stopped. A binary that will not run, a budget that ran out, a
// config obey could not load and a report that would not parse are all
// unknown: each of them is a machine where a daemon may well be serving, and
// grading them as stopped is what turns a failed probe into the claim that
// this install started the daemon.
func obeyDaemonStateAt(ctx context.Context, obeyPath string) daemonState {
	if _, err := os.Stat(obeyPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Nothing managed is installed, so nothing managed is serving.
			// This is the fresh-install case, and it is a real answer rather
			// than a failure to read one.
			return daemonStateStopped
		}
		return daemonStateUnknown
	}
	runCtx, cancel := context.WithTimeout(ctx, verbTimeout(serviceVerbStatus))
	defer cancel()
	cmd := exec.CommandContext(runCtx, obeyPath, "service", serviceVerbStatus, "--json") //nolint:gosec // obeyPath is the managed bin dir, args are constants
	cmd.Stdin = nil
	cmd.WaitDelay = serviceWaitDelay
	out, err := cmd.Output()
	if err != nil {
		return daemonStateUnknown
	}
	var rep serviceReport
	if err := json.Unmarshal(out, &rep); err != nil {
		return daemonStateUnknown
	}
	if rep.SocketReachable || rep.DaemonPID > 0 {
		return daemonStateRunning
	}
	return daemonStateStopped
}

// serviceContract is what the installer could establish about the obey it just
// staged, before driving any service verb through it.
type serviceContract struct {
	// Supported is true when both halves of the contract are present.
	Supported bool
	// Version is the obey the probe read, empty when it could not be read.
	Version string
	// Reason names what is missing, in the words the warning carries.
	Reason string
}

// obeyServiceContract checks the staged obey against the contract these steps
// are written for: `obey service restart` exists, and `obey service install`
// leaves a live daemon alone unless it is asked to replace it (obey #381).
//
// Against an obey without them every sentence this step prints about a live
// daemon is wrong. --no-restart would report a deferred restart that obey's
// own install had already performed, losing the live sessions the flag exists
// to keep; the default path would call a verb that command does not have, and
// a cobra parent with no run function answers an unknown subcommand by
// printing its help and exiting 0, so even the failure would be silent. The
// caller therefore withholds the verbs from an obey below the contract
// wherever a daemon may be serving (internal/app/obey.go).
func obeyServiceContract(ctx context.Context) serviceContract {
	obeyPath, err := obeyServicePath(ctx)
	if err != nil {
		return serviceContract{Reason: "the managed obey could not be located: " + err.Error()}
	}
	return obeyServiceContractAt(ctx, obeyPath)
}

// obeyServiceContractAt is obeyServiceContract asked of an explicit binary, so
// the probe and its tests grade the obey in front of them rather than whichever
// one the managed bin dir holds.
//
// The version comes from the one probe table every other reader uses
// (internal/app/statusreport.go), so the subcommand-then-flag fallback, the
// per-attempt budget and the parser are shared rather than restated here. An
// unreadable version is empty, and an empty version is not a refusal: the verb
// probes below still decide, because an obey that will not report its version
// can still list the commands it has.
func obeyServiceContractAt(ctx context.Context, obeyPath string) serviceContract {
	version, verr := probeToolVersion(ctx, obeyBinary, obeyPath)
	if verr != nil {
		version = ""
	}
	if version != "" && installer.VersionLess(releaseCore(version), obeyServiceContractFloor) {
		return serviceContract{
			Version: version,
			Reason: "obey " + version + " is below " + obeyServiceContractFloor +
				", the first release that supervises the daemon",
		}
	}
	help, err := obeyHelp(ctx, obeyPath, "service", "--help")
	if err != nil {
		return serviceContract{Version: version, Reason: "its obey service commands could not be read: " + err.Error()}
	}
	if !helpListsCommand(help, serviceVerbRestart) {
		return serviceContract{Version: version, Reason: "it has no obey service restart"}
	}
	installHelp, err := obeyHelp(ctx, obeyPath, "service", serviceVerbInstall, "--help")
	if err != nil {
		return serviceContract{Version: version, Reason: "its obey service install flags could not be read: " + err.Error()}
	}
	if !helpListsFlag(installHelp, serviceInstallRestartFlag) {
		return serviceContract{
			Version: version,
			Reason: "its obey service install has no " + serviceInstallRestartFlag +
				" flag, so it replaces a running daemon on its own",
		}
	}
	return serviceContract{Supported: true, Version: version}
}

// obeyHelp reads one obey help screen. --help is answered before the command
// body runs and before obey loads its config, so this asks the binary what it
// can do without asking it to do anything.
func obeyHelp(ctx context.Context, obeyPath string, args ...string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, serviceTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, obeyPath, args...) //nolint:gosec // obeyPath is the managed bin dir, args are constants
	cmd.Stdin = nil
	cmd.WaitDelay = serviceWaitDelay
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return "", errpkg.Wrap(codeServiceContract, err, "obey "+strings.Join(args, " ")+": "+detail)
	}
	return string(out), nil
}

// helpListsCommand reports whether a help screen lists name as a subcommand.
// An exit code cannot answer this, which is why the text is read: cobra prints
// the parent's help and exits 0 for a subcommand the parent does not have, so
// `obey service restart --help` succeeds against an obey with no restart verb.
// Only the indented lines count, so the usage line cannot pass for a command.
func helpListsCommand(help, name string) bool {
	for _, line := range strings.Split(help, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == line {
			continue
		}
		if trimmed == name || strings.HasPrefix(trimmed, name+" ") || strings.HasPrefix(trimmed, name+"\t") {
			return true
		}
	}
	return false
}

// helpListsFlag reports whether a help screen declares name as a flag of the
// command it describes. Only the indented flag lines count, and only the flag
// names on them: a cobra screen carries the command's long description above
// the usage block, and obey's own install help names --restart in that prose,
// so a match anywhere on the screen reads a sentence about the flag as the
// flag itself. The description that follows a flag is cut off at the two
// spaces cobra separates it with, so one flag documented in terms of another
// cannot stand in for it either.
func helpListsFlag(help, name string) bool {
	for _, line := range strings.Split(help, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == line || !strings.HasPrefix(trimmed, "-") {
			continue
		}
		names := trimmed
		if i := strings.Index(names, "  "); i >= 0 {
			names = names[:i]
		}
		for _, field := range strings.FieldsFunc(names, isFlagNameSeparator) {
			if field == name {
				return true
			}
		}
	}
	return false
}

// isFlagNameSeparator splits a cobra flag line's name column into the names it
// declares: a shorthand and a long name, and the type placeholder after them.
func isFlagNameSeparator(r rune) bool {
	return r == ' ' || r == '\t' || r == ',' || r == '='
}

// releaseCore drops a prerelease or build suffix before the floor comparison,
// so 0.1.0-rc.1 is graded as the 0.1.0 release it is a candidate for rather
// than refused for its suffix. The floor asks whether an obey predates the
// contract, and a prerelease of the floor release does not.
func releaseCore(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return v
}

// obeyServicePath is the managed obey binary the service verbs run through.
// Always the absolute staged path, never a PATH lookup: the point of the step
// is to register the binary this install just placed.
func obeyServicePath(ctx context.Context) (string, error) {
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(binDir, obeyBinary), nil
}

// ServiceNoteFor is the human sentence for an install's service step, exported
// so the CLI renders the same words the JSON warnings carry.
func ServiceNoteFor(res InstallResult) string {
	return serviceNote(res.Service, res.Version, false)
}

// serviceNote is the one sentence a user needs when the service step did not
// leave the daemon running the version that is now on disk.
//
// reportStarted says whether a started daemon is news on this path. An update
// that finds the daemon stopped and leaves it running has changed something
// the caller did not ask about; a fresh install that brings it up did exactly
// what it was asked. The ServiceResult field is set either way, because an
// agent reading the payload has no way to know the sentence is update-only.
func serviceNote(svc *ServiceResult, version string, reportStarted bool) string {
	switch {
	case svc == nil:
		return ""
	case svc.Unsupported:
		return "obey " + version + " is installed; this obey predates the supervised daemon service (" +
			svc.ContractReason + "), so no service step ran. Register the service and restart the daemon " +
			"yourself with: obey service install"
	case svc.Deferred && svc.DaemonStateUnknown:
		return "obey " + version + " is installed; whether a daemon was already running could not be determined, " +
			"so nothing was restarted. Check it with: obey service status"
	case svc.Deferred:
		return "obey " + version + " is installed; the running daemon is still the previous version. Restart it with: obey service restart"
	case svc.TimedOut:
		return svc.Error + ". The binaries are installed and the supervisor may still be finishing it; check with: obey service status"
	case svc.Error != "" && svc.Installed:
		return "obey service step failed: " + svc.Error + ". The binaries are installed and the service is registered; the running daemon is still the previous version."
	case svc.Error != "":
		return "obey service step failed: " + svc.Error + ". The binaries are installed; the daemon will not start automatically."
	case svc.Started && reportStarted:
		return "obey " + version + " is installed; no daemon was running, so it was started on the new version rather than restarted"
	default:
		return ""
	}
}

// appendWarning joins two human warnings the way UpdateFestival does.
func appendWarning(existing, add string) string {
	switch {
	case add == "":
		return existing
	case existing == "":
		return add
	default:
		return existing + "; " + add
	}
}
