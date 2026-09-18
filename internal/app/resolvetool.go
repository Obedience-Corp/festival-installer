package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// ResolveTool finds the absolute path to a suite tool by name.
//
// Package and leftover origins use LookPath first so the hub runs the suite
// the shell would run. A leftover PATH plus a stale ~/.obey/installer/bin
// must not hide the live copies. Managed origin still prefers the managed
// bin when that file exists (just-installed hub copy, managed bin not on
// PATH yet).
//
// The error codes are E_LAUNCH_TOOL and E_LAUNCH_NOT_FOUND, unchanged from when
// this lived in the launch package, because callers match on them.
//
// This is the single-tool entry point and it detects the suite itself. A caller
// resolving several tools in one pass should detect once and use
// resolveToolFrom, because DetectSuite re-execs every camp, fest, and festival
// on PATH each time it runs.
func ResolveTool(ctx context.Context, tool string) (string, error) {
	name, err := checkToolName(ctx, tool)
	if err != nil {
		return "", err
	}
	origin, _ := DetectSuite(ctx)
	return resolveToolFrom(ctx, name, origin.Kind)
}

// resolveToolFrom is ResolveTool's decision with the suite origin already in
// hand. Only the origin kind is read, so one DetectSuite serves every tool in a
// report instead of one per tool.
func resolveToolFrom(ctx context.Context, tool string, kind OriginKind) (string, error) {
	name, err := checkToolName(ctx, tool)
	if err != nil {
		return "", err
	}

	managed := managedToolPath(ctx, name)
	preferPath := kind == OriginPackage || kind == OriginLeftover
	if preferPath {
		if path, lerr := exec.LookPath(name); lerr == nil {
			return path, nil
		}
		if managed != "" {
			return managed, nil
		}
	} else {
		if managed != "" {
			return managed, nil
		}
		if path, lerr := exec.LookPath(name); lerr == nil {
			return path, nil
		}
	}

	return "", errpkg.New("E_LAUNCH_NOT_FOUND",
		name+" not found in managed bin or PATH (install the suite or fix PATH from the hub)")
}

// checkToolName rejects a cancelled context and anything that is not a bare
// binary name. It runs before detection so an invalid name costs no execs.
func checkToolName(ctx context.Context, tool string) (string, error) {
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
	return tool, nil
}

// ManagedToolPath is <bin-dir>/<tool> when that file exists, and "" otherwise.
// It is the managed half of what ResolveTool decides between.
func ManagedToolPath(ctx context.Context, tool string) string { return managedToolPath(ctx, tool) }

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
