package app

import (
	"os"
	"path/filepath"
)

func dirOnPath(binDir string) bool {
	target := resolvePath(binDir)
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == "" {
			continue
		}
		if resolvePath(entry) == target {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return resolvePath(a) == resolvePath(b)
}

func resolvePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return resolved
	}
	return filepath.Clean(p)
}
