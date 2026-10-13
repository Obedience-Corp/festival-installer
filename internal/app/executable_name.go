package app

import (
	"runtime"
	"strings"
)

// executableName is the on-disk name for a product's declared command name.
// Marketplace metadata stays platform-neutral; Windows archives carry .exe.
func executableName(name string) string { return executableNameForOS(name, runtime.GOOS) }

func executableNameForOS(name, goos string) string {
	if goos == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}
