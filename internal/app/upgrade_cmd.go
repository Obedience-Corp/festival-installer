package app

import (
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/installer"
)

// PackageUpgradeAvailable reports whether a package-origin update found a
// newer suite than the one on PATH.
func PackageUpgradeAvailable(res UpdateResult) bool {
	if res.Action != "package" {
		return false
	}
	if res.Version == "" || res.Latest == "" {
		return false
	}
	return installer.VersionLess(stripVersionPrefix(res.Version), stripVersionPrefix(res.Latest))
}

// ParseUpgradeArgv splits a package-manager upgrade line into a binary and
// args the hub can exec. Shell pipelines, URLs, and prose ("reinstall X from")
// return ok=false so the TUI/CLI keep printing the line instead of running it.
func ParseUpgradeArgv(cmd string) (tool string, args []string, ok bool) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "", nil, false
	}
	if strings.ContainsAny(cmd, "|&;<>") {
		return "", nil, false
	}
	if strings.HasPrefix(cmd, "http://") || strings.HasPrefix(cmd, "https://") {
		return "", nil, false
	}
	if strings.HasPrefix(cmd, "reinstall ") {
		return "", nil, false
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "", nil, false
	}
	tool = fields[0]
	if strings.Contains(tool, "/") {
		return "", nil, false
	}
	return tool, fields[1:], true
}
