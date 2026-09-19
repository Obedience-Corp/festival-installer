package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// ResolveTool finds the absolute path to a suite tool by name.
//
// Package origin (including Dual package) uses LookPath first so the hub runs
// the suite the shell would run.
//
// Leftover origin has no authoritative copy. ~/.obey/installer/bin can hold a
// stale hub install while PATH holds the live one, and it can equally hold a
// just-installed hub copy while PATH still has the old leftover. Nothing about
// either path tells those two apart, so the newer version wins, and anything
// short of a readable version on both sides that puts PATH ahead keeps the
// managed copy, which is the one the hub controls.
//
// Managed origin prefers the managed bin.
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
	path, _, err := resolveToolFrom(ctx, name, origin)
	return path, err
}

// resolveToolFrom is ResolveTool's decision with the suite origin already in
// hand, so one DetectSuite serves every tool in a report instead of one per
// tool. It returns the version it read while deciding, empty when the decision
// needed none, so a caller that wants the version does not exec the same binary
// a second time.
func resolveToolFrom(ctx context.Context, tool string, origin SuiteOrigin) (string, string, error) {
	name, err := checkToolName(ctx, tool)
	if err != nil {
		return "", "", err
	}

	managed := managedToolPath(ctx, name)
	onPATH := ""
	if p, lerr := exec.LookPath(name); lerr == nil {
		onPATH = p
	}

	switch {
	case origin.Kind == OriginPackage:
		if onPATH != "" {
			return onPATH, "", nil
		}
		if managed != "" {
			return managed, "", nil
		}
	case origin.Kind == OriginLeftover && onPATH != "" && managed != "" && !samePath(onPATH, managed):
		path, version := newerCopy(ctx, name, onPATH, origin.versionAt(onPATH), managed)
		return path, version, nil
	default:
		if managed != "" {
			return managed, "", nil
		}
		if onPATH != "" {
			return onPATH, "", nil
		}
	}

	return "", "", errpkg.New("E_LAUNCH_NOT_FOUND",
		name+" not found in managed bin or PATH (install the suite or fix PATH from the hub)")
}

// newerCopy picks between a leftover PATH copy and the managed bin by version,
// and returns the version of whichever it picked.
//
// The PATH copy's version comes from the suite detection that already probed
// it, so only the managed copy costs an exec, and only when both copies exist.
// installer.VersionLess reads a `git describe` suffix as ahead of its tag
// rather than below it, which is what camp and fest stamp. The managed copy
// wins a tie and wins whenever either side will not report a version: the hub
// installed that one, so it is the copy to fall back on when the comparison
// cannot be made.
func newerCopy(ctx context.Context, tool, onPATH, pathVersion, managed string) (string, string) {
	if pathVersion == "" {
		pathVersion, _ = probeToolVersion(ctx, tool, onPATH)
	}
	managedVersion, _ := probeToolVersion(ctx, tool, managed)
	if pathVersion != "" && managedVersion != "" && installer.VersionLess(managedVersion, pathVersion) {
		return onPATH, pathVersion
	}
	return managed, managedVersion
}

// versionAt is the version DetectSuite already probed for the copy at path, or
// empty when detection never saw it. obey and ob are never in the suite tables,
// so they always report empty here.
func (o SuiteOrigin) versionAt(path string) string {
	for _, list := range [][]ToolLocation{o.Tools, o.Shadows} {
		for _, c := range list {
			if samePath(c.Path, path) {
				return c.Version
			}
		}
	}
	return ""
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
