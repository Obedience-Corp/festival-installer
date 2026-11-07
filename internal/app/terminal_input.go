package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
)

// Stage the optional Linux host integration under the same transaction and
// receipt as obey/ob. Older archives have neither file; a partial pair is an
// invalid release and must not result in a successful install receipt.
func stageTerminalInputPayload(ctx context.Context, tx *installer.Transaction, packageID, extractDir, binDir string) ([]string, error) {
	if runtime.GOOS != "linux" || packageID != ObeyPackageID {
		return nil, nil
	}
	names := []string{"obey-pty-acquire", "install-terminal-input.sh"}
	present := 0
	for _, name := range names {
		info, err := os.Lstat(filepath.Join(extractDir, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("invalid Linux terminal input payload %s", name)
		}
		present++
	}
	if present == 0 {
		return nil, nil
	}
	if present != len(names) {
		return nil, fmt.Errorf("obey archive has an incomplete Linux terminal input payload; helper and installer must ship together")
	}
	var files []string
	for _, name := range names {
		source := filepath.Join(extractDir, name)
		hash, err := artifacts.SHA256(ctx, source)
		if err != nil {
			return nil, err
		}
		destination := filepath.Join(binDir, "libexec", "obey", name)
		if err := tx.Stage(ctx, installer.StagedFile{StagedPath: source, DestPath: destination, Sha256: hash, Mode: 0o755}); err != nil {
			return nil, err
		}
		files = append(files, destination)
	}
	return files, nil
}

type terminalInputSetupReport struct {
	Schema  string `json:"schema"`
	Status  string `json:"status"`
	Ready   bool   `json:"ready"`
	Changed bool   `json:"changed"`
	Detail  string `json:"detail"`
}

func isTerminalInputPayloadPath(path, binDir string) bool {
	name := filepath.Base(path)
	if name != "obey-pty-acquire" && name != "install-terminal-input.sh" {
		return false
	}
	// Comparing with a literal child of the resolved bin directory also refuses
	// a symlinked payload parent that would escape the managed tree.
	return resolvePath(filepath.Dir(path)) == filepath.Join(resolvePath(binDir), "libexec", "obey")
}

// Inspection is always noninteractive. Foreground setup is the only caller
// allowed to request the narrow helper's administrator authorization.
func reconcileTerminalInput(ctx context.Context, r *IntegrationReport, check bool) {
	if runtime.GOOS != "linux" {
		return
	}
	obeyPath, err := ResolveTool(ctx, obeyBinary)
	if err != nil {
		return
	} // The existing provider integration reports absent Obey.
	args := []string{"terminal-input", "setup", "--json"}
	if check {
		args = append(args, "--check")
	} else {
		args = append(args, "--authorize")
	}
	budget := 10 * time.Minute
	if check {
		budget = 5 * time.Second
	}
	run, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	command := exec.CommandContext(run, obeyPath, args...) //nolint:gosec // resolved owning binary; arguments fixed
	// The child owns any graphical authorization dialog. Never pipe a password
	// into it or mix the child's installer output into the parent's JSON stdout.
	if !check {
		command.Stderr = os.Stderr
		command.Stdin = os.Stdin
	}
	raw, err := command.Output()
	if err != nil {
		// A legacy Obey may predate this setup contract. It cannot claim the new
		// feature is configured; updating Obey remains the actionable remediation.
		r.add("terminal_input", "upgrade_required", "Existing-terminal input could not be checked. Update or repair Obey, then retry terminal setup on this Linux host.", obeyPath, true)
		return
	}
	var report terminalInputSetupReport
	if err := json.Unmarshal(raw, &report); err != nil || report.Schema != "obey-terminal-input/v1" || report.Status == "" || report.Detail == "" {
		r.add("terminal_input", "upgrade_required", "Update Obey to a version that reports Linux terminal input setup status.", obeyPath, true)
		return
	}
	status := report.Status
	if status == "payload_missing" {
		status = "upgrade_required"
	}
	if report.Ready {
		status = "ready"
	}
	r.add("terminal_input", status, report.Detail, obeyPath, !report.Ready)
	r.Changed = r.Changed || report.Changed
}

// Repair an interrupted/older installer placement even when the advertised
// version is current. Legacy Obey without this protocol is not guessed at.
func needsTerminalInputPayloadRepair(ctx context.Context) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	path, err := obeyServicePath(ctx)
	if err != nil {
		return false
	}
	directory := filepath.Join(filepath.Dir(path), "libexec", "obey")
	complete := true
	for _, name := range []string{"obey-pty-acquire", "install-terminal-input.sh"} {
		info, err := os.Lstat(filepath.Join(directory, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			complete = false
		}
	}
	if complete {
		return false
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(probe, path, "terminal-input", "setup", "--json", "--check").Output() //nolint:gosec // managed binary and fixed arguments
	var report terminalInputSetupReport
	return err == nil && json.Unmarshal(raw, &report) == nil && report.Schema == "obey-terminal-input/v1"
}
