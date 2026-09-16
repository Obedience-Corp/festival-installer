package app

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// serviceTimeout bounds one obey service verb. launchctl bootstrap and
// systemctl --user both answer in well under a second on a healthy machine; the
// budget exists so a wedged supervisor cannot hold the app's launch.
const serviceTimeout = 30 * time.Second

// Service verbs, matching the commands obey registers. They are constants
// because the installer calls them by name and reads only the exit code, so a
// rename in obey is a one-line change here.
const (
	serviceVerbInstall   = "install"
	serviceVerbRestart   = "restart"
	serviceVerbUninstall = "uninstall"
)

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
	runCtx, cancel := context.WithTimeout(ctx, serviceTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, obeyPath, "service", verb) //nolint:gosec // obeyPath is the managed bin dir, verb is a constant
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return errpkg.Wrap("E_SERVICE_VERB", err, "obey service "+verb+": "+detail)
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
	case svc.Error != "":
		return "obey service step failed: " + svc.Error + ". The binaries are installed; the daemon will not start automatically."
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
