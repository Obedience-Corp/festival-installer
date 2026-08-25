package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// OriginKind is how the active suite got onto this machine.
// Distinct from ValidateChannel (stable|rc|dev).
type OriginKind string

const (
	OriginAbsent   OriginKind = "absent"
	OriginManaged  OriginKind = "managed"  // active LookPath is state.BinDir
	OriginPackage  OriginKind = "package"  // aur, homebrew, npm, deb, rpm, apk, install.sh
	OriginLeftover OriginKind = "leftover" // go install, ~/local/bin, unknown
)

// PackageFlavor refines OriginPackage. Empty for other kinds.
type PackageFlavor string

const (
	FlavorAUR       PackageFlavor = "aur"
	FlavorHomebrew  PackageFlavor = "homebrew"
	FlavorNpm       PackageFlavor = "npm"
	FlavorDeb       PackageFlavor = "deb"
	FlavorRpm       PackageFlavor = "rpm"
	FlavorApk       PackageFlavor = "apk"
	FlavorInstallSh PackageFlavor = "install.sh"
	FlavorUnknown   PackageFlavor = "unknown"
)

// suiteTools is the PATH names DetectSuite enumerates. camp is preferred for
// Kind; fest then festival are fallbacks for the active LookPath.
var suiteTools = []string{"camp", "fest", selfBinaryName}

type ToolLocation struct {
	Tool    string        `json:"tool"`
	Path    string        `json:"path"`
	Version string        `json:"version,omitempty"`
	Bundle  string        `json:"bundle,omitempty"`
	Origin  OriginKind    `json:"origin"`
	Flavor  PackageFlavor `json:"flavor,omitempty"`
}

type SuiteOrigin struct {
	Kind       OriginKind     `json:"kind"`
	Flavor     PackageFlavor  `json:"flavor,omitempty"`
	Prefix     string         `json:"prefix,omitempty"`
	Helper     string         `json:"helper,omitempty"`
	Upgrade    string         `json:"upgrade,omitempty"`
	Remove     string         `json:"remove,omitempty"`
	Version    string         `json:"version,omitempty"`
	RelChannel string         `json:"rel_channel,omitempty"`
	Package    string         `json:"package,omitempty"`
	Tools      []ToolLocation `json:"tools,omitempty"`
	Shadows    []ToolLocation `json:"shadows,omitempty"`
	Dual       bool           `json:"dual,omitempty"`
}

// DetectSuite classifies the on-PATH camp/fest/festival suite.
//
// Read-only: does not create installer home or open the receipt database.
// Missing $FESTIVAL_HOME is not an error. PATH classification errors still
// return a best-effort origin.
func DetectSuite(ctx context.Context) (SuiteOrigin, error) {
	origin := SuiteOrigin{Kind: OriginAbsent}
	if err := ctx.Err(); err != nil {
		return origin, errpkg.Wrap("E_DETECT_CTX", err, "context cancelled")
	}

	copies, walkErr := enumerateSuiteCopies()
	active := activeSuitePath()
	if active != "" {
		kind, flavor := classifyPath(ctx, active)
		origin.Kind = kind
		origin.Flavor = flavor
		origin.Prefix = filepath.Dir(active)
		origin.Tools = activeTools(copies, origin.Prefix, kind, flavor)
		origin.Shadows = shadowTools(copies, origin.Prefix)
	}

	dual, dualErr := dualFromBinDir(ctx, origin.Kind, copies)
	if dualErr != nil && errpkg.Code(dualErr) == "E_HOME_NOT_ABS" {
		dualErr = nil
	}
	origin.Dual = dual

	if walkErr != nil {
		return origin, walkErr
	}
	return origin, dualErr
}

// classifyPath is a stub: managed if the file sits in BinDir and home exists,
// otherwise leftover. Full helper-adjacent / Homebrew / npm rules are the next
// task.
func classifyPath(ctx context.Context, path string) (OriginKind, PackageFlavor) {
	if path == "" {
		return OriginLeftover, ""
	}
	exists, err := state.HomeExists(ctx)
	if err != nil || !exists {
		return OriginLeftover, ""
	}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return OriginLeftover, ""
	}
	if samePath(filepath.Dir(path), binDir) {
		return OriginManaged, ""
	}
	return OriginLeftover, ""
}

func activeSuitePath() string {
	for _, tool := range suiteTools {
		if p, err := exec.LookPath(tool); err == nil && p != "" {
			return p
		}
	}
	return ""
}

func enumerateSuiteCopies() ([]ToolLocation, error) {
	var out []ToolLocation
	seen := map[string]struct{}{}
	var firstErr error
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		for _, tool := range suiteTools {
			p := filepath.Join(dir, tool)
			st, err := os.Stat(p)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				if firstErr == nil {
					firstErr = errpkg.Wrap("E_DETECT_PATH", err, "stat "+p)
				}
				continue
			}
			if st.IsDir() {
				continue
			}
			key := resolvePath(p)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, ToolLocation{Tool: tool, Path: p})
		}
	}
	return out, firstErr
}

func activeTools(copies []ToolLocation, prefix string, kind OriginKind, flavor PackageFlavor) []ToolLocation {
	var out []ToolLocation
	for _, c := range copies {
		if samePath(filepath.Dir(c.Path), prefix) {
			c.Origin = kind
			c.Flavor = flavor
			out = append(out, c)
		}
	}
	return out
}

func shadowTools(copies []ToolLocation, prefix string) []ToolLocation {
	var out []ToolLocation
	for _, c := range copies {
		if samePath(filepath.Dir(c.Path), prefix) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func dualFromBinDir(ctx context.Context, kind OriginKind, copies []ToolLocation) (bool, error) {
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return false, err
	}
	hasManaged := binDirHasSuite(binDir)
	if !hasManaged {
		return false, nil
	}
	if kind != OriginManaged {
		return true, nil
	}
	for _, c := range copies {
		if !samePath(filepath.Dir(c.Path), binDir) {
			return true, nil
		}
	}
	return false, nil
}

func binDirHasSuite(binDir string) bool {
	for _, tool := range suiteTools {
		st, err := os.Stat(filepath.Join(binDir, tool))
		if err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}
