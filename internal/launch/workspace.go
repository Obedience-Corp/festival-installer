package launch

import (
	"os"
	"path/filepath"
	"strings"
)

// DetectCampaignRoot walks up from start (or the process cwd when empty) looking
// for a .campaign directory. Returns "" when none is found.
func DetectCampaignRoot(start string) string {
	dir := strings.TrimSpace(start)
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ""
		}
		dir = wd
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, ".campaign")); err == nil && st.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// WantsCampaignRoot reports whether the launch should prefer campaign root as cwd.
func WantsCampaignRoot(spec Spec) bool {
	switch spec.Tool {
	case "camp":
		if len(spec.Args) == 0 {
			return false
		}
		switch spec.Args[0] {
		case "wi", "intent", "go", "workitem", "workitems":
			return true
		}
	case "fest":
		if len(spec.Args) == 0 {
			return false
		}
		switch spec.Args[0] {
		case "list", "watch", "status", "next", "go":
			return true
		}
	}
	return false
}
