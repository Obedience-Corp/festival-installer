package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state"
)

// ResolveWhich finds the active and managed locations for a tool binary.
func ResolveWhich(ctx context.Context, tool string) (WhichResult, error) {
	r := WhichResult{Tool: tool}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return r, err
	}
	managed := filepath.Join(binDir, tool)
	switch fi, statErr := os.Stat(managed); {
	case statErr == nil:
		if !fi.IsDir() {
			r.Managed = managed
		}
	case !os.IsNotExist(statErr):
		return r, errpkg.Wrap("E_MANAGED_STAT", statErr, "stat managed binary "+managed)
	}
	if active, err := exec.LookPath(tool); err == nil {
		r.Path = active
		r.OnPath = true
	}
	if r.Path == "" && r.Managed == "" {
		return r, errpkg.New("E_TOOL_NOT_FOUND",
			"no "+tool+" binary found on PATH or in the managed bin directory")
	}
	if r.Path != "" && r.Managed != "" {
		r.Shadowed = !samePath(r.Path, r.Managed)
	}
	return r, nil
}
