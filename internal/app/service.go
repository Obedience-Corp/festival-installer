package app

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// serviceTimeout bounds the supervisor verbs. launchctl bootstrap, its removal
// counterpart and a status read all answer in well under a second on a healthy
// machine; the budget exists so a wedged supervisor cannot hold the app's
// launch.
const serviceTimeout = 30 * time.Second

// serviceRestartTimeout bounds the restart verb, which is not a supervisor call
// but a process swap. obey drains for up to 10s on SIGTERM, stops its gRPC
// server gracefully, then waits up to 20s for the replacement to answer, so a
// budget sized for install reports a legitimate restart as a failure and kills
// obey in the middle of it.
const serviceRestartTimeout = 90 * time.Second

// codeServiceTimeout marks a verb the installer cut short rather than one the
// supervisor refused. The two need different words: a refusal means the daemon
// is not coming up, a timeout means nobody knows yet.
const codeServiceTimeout = "E_SERVICE_TIMEOUT"

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
	// running before, rather than restarting one that was.
	Started bool `json:"started,omitempty"`
}

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

// obeyDaemonRunning reports whether a daemon is already up, asked of the obey
// binary that is on disk right now. It is the question install has to answer
// before it replaces that binary: a daemon that was running keeps running the
// old image until something restarts it. Any failure is a no, which is the
// right answer for the fresh-install case where there is no obey to ask.
func obeyDaemonRunning(ctx context.Context) bool {
	obeyPath, err := obeyServicePath(ctx)
	if err != nil {
		return false
	}
	runCtx, cancel := context.WithTimeout(ctx, verbTimeout(serviceVerbStatus))
	defer cancel()
	cmd := exec.CommandContext(runCtx, obeyPath, "service", serviceVerbStatus, "--json") //nolint:gosec // obeyPath is the managed bin dir, args are constants
	cmd.Stdin = nil
	cmd.WaitDelay = serviceWaitDelay
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	var rep serviceReport
	if err := json.Unmarshal(out, &rep); err != nil {
		return false
	}
	return rep.SocketReachable || rep.DaemonPID > 0
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
	return serviceNote(res.Service, res.Version)
}

// serviceNote is the one sentence a user needs when the service step did not
// leave the daemon running the version that is now on disk.
func serviceNote(svc *ServiceResult, version string) string {
	switch {
	case svc == nil:
		return ""
	case svc.Deferred:
		return "obey " + version + " is installed; the running daemon is still the previous version. Restart it with: obey service restart"
	case svc.TimedOut:
		return svc.Error + ". The binaries are installed and the supervisor may still be finishing it; check with: obey service status"
	case svc.Error != "" && svc.Installed:
		return "obey service step failed: " + svc.Error + ". The binaries are installed and the service is registered; the running daemon is still the previous version."
	case svc.Error != "":
		return "obey service step failed: " + svc.Error + ". The binaries are installed; the daemon will not start automatically."
	case svc.Started:
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
