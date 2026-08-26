package launch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// Resolve finds the absolute path to tool.
//
// Package origin (including Dual package) uses LookPath first so launchpad
// matches the suite the shell would run. Otherwise prefer the managed bin
// when that file exists, even if PATH still has leftovers (just-installed
// hub copy, managed bin not on PATH yet).
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

	managed := managedToolPath(ctx, tool)
	origin, _ := app.DetectSuite(ctx)
	if origin.Kind == app.OriginPackage {
		if path, err := exec.LookPath(tool); err == nil {
			return path, nil
		}
		if managed != "" {
			return managed, nil
		}
	} else {
		if managed != "" {
			return managed, nil
		}
		if path, err := exec.LookPath(tool); err == nil {
			return path, nil
		}
	}

	return "", errpkg.New("E_LAUNCH_NOT_FOUND",
		tool+" not found in managed bin or PATH (install the suite or fix PATH from the hub)")
}

func managedToolPath(ctx context.Context, tool string) string {
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return ""
	}
	p := filepath.Join(binDir, tool)
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return ""
	}
	return p
}
