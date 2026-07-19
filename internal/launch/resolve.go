package launch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state"
)

// Resolve finds the absolute path to tool.
// Prefer the installer-managed bin when the binary exists there; else PATH.
func Resolve(ctx context.Context, tool string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return "", errpkg.New("E_LAUNCH_TOOL", "empty tool name")
	}
	if strings.Contains(tool, string(os.PathSeparator)) || strings.Contains(tool, "/") {
		return "", errpkg.New("E_LAUNCH_TOOL", "tool must be a bare binary name")
	}

	if binDir, err := state.BinDir(ctx); err == nil {
		managed := filepath.Join(binDir, tool)
		if fi, statErr := os.Stat(managed); statErr == nil && !fi.IsDir() {
			return managed, nil
		}
	}

	path, err := exec.LookPath(tool)
	if err != nil {
		return "", errpkg.Wrap("E_LAUNCH_NOT_FOUND", err,
			tool+" not found in managed bin or PATH (install the suite or fix PATH from the hub)")
	}
	return path, nil
}
