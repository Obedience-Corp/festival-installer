package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// probeTimeout is what one probe attempt gets. It carries headroom for a
	// first exec on a busy machine rather than being tuned to a warm one: at
	// load average 182 on 16 cores, a 2s attempt was exceeded repeatedly,
	// including the first exec of a 73MB obey. A missed budget reports no
	// version, and an absent version reads as below the app's floor.
	probeTimeout = 5 * time.Second
	// probeWaitDelay bounds the wait for a probe's stdout to reach EOF after
	// its deadline. exec kills the direct child when the context expires, but
	// Output then waits for the pipe, and a grandchild that inherited stdout
	// holds it open for as long as it lives: without this, a 2s context
	// returned after 30s against the fixture in
	// TestProbes_BoundedWhenAChildHoldsStdout.
	probeWaitDelay = 250 * time.Millisecond
	flavorTimeout  = 200 * time.Millisecond
	releasesURL    = "https://github.com/Obedience-Corp/festival/releases/latest"
	docsInstall    = "https://docs.fest.build/getting-started/installation/"
	installShURL   = "https://raw.githubusercontent.com/Obedience-Corp/festival/main/install.sh"
)

func flavorFromPackageManager(ctx context.Context, path string) (PackageFlavor, string) {
	if path == "" {
		return "", ""
	}
	if out, ok := runTimed(ctx, flavorTimeout, "pacman", "-Qqo", path); ok {
		pkg := strings.TrimSpace(out)
		if pkg == "festival-bin" {
			return FlavorAUR, pkg
		}
	}
	if out, ok := runTimed(ctx, flavorTimeout, "dpkg", "-S", path); ok {
		pkg := dpkgPackage(out)
		if pkg != "" {
			return FlavorDeb, pkg
		}
	}
	if out, ok := runTimed(ctx, flavorTimeout, "rpm", "-qf", path); ok {
		pkg := strings.TrimSpace(out)
		if pkg != "" && !strings.Contains(pkg, "not owned") {
			return FlavorRpm, pkg
		}
	}
	if out, ok := runTimed(ctx, flavorTimeout, "apk", "info", "-W", path); ok {
		pkg := strings.TrimSpace(out)
		if pkg != "" {
			return FlavorApk, pkg
		}
	}
	return "", ""
}

func dpkgPackage(out string) string {
	line := strings.TrimSpace(out)
	if i := strings.IndexByte(line, ':'); i > 0 {
		return strings.TrimSpace(line[:i])
	}
	return ""
}

func runTimed(ctx context.Context, d time.Duration, name string, args ...string) (string, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = probeWaitDelay
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return buf.String(), true
}

func probeCopies(ctx context.Context, copies []ToolLocation) []ToolLocation {
	if len(copies) == 0 {
		return copies
	}
	out := make([]ToolLocation, len(copies))
	copy(out, copies)
	var wg sync.WaitGroup
	for i := range out {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ver, bundle, profile := probeBinary(ctx, out[i].Path, out[i].Tool)
			out[i].Version = ver
			out[i].Bundle = bundle
			out[i].Profile = profile
		}(i)
	}
	wg.Wait()
	return out
}

func probeBinary(ctx context.Context, path, tool string) (version, bundle, profile string) {
	if path == "" {
		return "", "", ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if tool != selfBinaryName {
		if raw, err := probeOnce(ctx, path, "version", "--json"); err == nil {
			var parsed struct {
				Version string `json:"version"`
				Bundle  string `json:"bundle"`
				Profile string `json:"profile"`
			}
			// camp and fest stamp the git tag verbatim, so the JSON version
			// carries the same leading v the text output does.
			if json.Unmarshal(raw, &parsed) == nil {
				if v := ParseToolVersion(tool, parsed.Version); v != "" {
					return v, parsed.Bundle, parsed.Profile
				}
			}
		}
	}
	raw, err := probeOnce(ctx, path, "version")
	if err != nil {
		return "", "", ""
	}
	return parseVersionText(tool, string(raw))
}

// probeOnce gives one argument set the full probe budget. The budget is per
// attempt rather than shared across a tool's table, because the fallbacks exist
// precisely for binaries that reject the first argument set: a released
// festival rejects `version --short`, and under a shared budget its refusal,
// or just a cold first exec, can leave the fallback no time to answer.
func probeOnce(ctx context.Context, path string, args ...string) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	return runProbe(runCtx, path, args...)
}

// runProbe runs one version probe and returns its stdout. WaitDelay is what
// makes the caller's timeout real, and a grandchild still holding stdout is not
// a probe failure: the tool's own output was already written, so ErrWaitDelay
// is folded into success and parsed like any other.
func runProbe(ctx context.Context, path string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // path from ResolveTool or the managed bin dir, args from a fixed table
	cmd.Stdin = nil
	cmd.WaitDelay = probeWaitDelay
	out, err := cmd.Output()
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return nil, err
	}
	return out, nil
}

// parseVersionText reads a tool's `version` output: the version from the first
// line, and the bundle and profile rows when the tool prints them.
func parseVersionText(tool, text string) (version, bundle, profile string) {
	version = ParseToolVersion(tool, text)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "bundle:"); ok {
			rest = strings.TrimSpace(rest)
			rest = strings.TrimPrefix(rest, "festival")
			bundle = strings.TrimSpace(rest)
		}
		if rest, ok := strings.CutPrefix(line, "profile:"); ok {
			profile = strings.TrimSpace(rest)
		}
	}
	return version, bundle, profile
}

// ParseToolVersion reads what a suite tool printed for its version and returns
// the one shape every festival command reports: the bare semver core plus any
// pre-release or git-describe suffix, with no leading v and no tool name.
//
// Every form the shipped binaries print normalizes here. camp and fest stamp
// `git describe --tags` against vX.Y.Z tags, so `version --short` prints
// "v0.10.1-4-gd1fb37c7" and `version` prints that line prefixed with the tool
// name; festival prints the bare "v0.2.2"; cobra's default version template
// prints "obey version 0.1.0". Output that is not a version, a help banner for
// instance, returns "": a wrong version is worse for a floor check than an
// absent one.
func ParseToolVersion(tool, output string) string {
	v := firstLine(output)
	if rest, ok := strings.CutPrefix(v, tool+" "); ok {
		v = strings.TrimSpace(rest)
	}
	if rest, ok := strings.CutPrefix(v, "version "); ok {
		v = strings.TrimSpace(rest)
	}
	v = strings.TrimPrefix(v, "v")
	if !LooksLikeVersion(v) {
		return ""
	}
	return v
}

// firstLine is the first non-blank line, which keeps a multi-page help dump out
// of a reported version or error.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

func applyGuidance(origin *SuiteOrigin) {
	if origin == nil {
		return
	}
	helper := origin.Helper
	if helper == "" && origin.Prefix != "" {
		if h := helperFile(helperDir(origin.Prefix)); h != "" {
			helper = h
			origin.Helper = h
		}
	}
	switch origin.Kind {
	case OriginManaged:
		origin.Helper = ""
		origin.Upgrade = "festival update"
		origin.Remove = "festival uninstall festival"
	case OriginPackage:
		applyPackageGuidance(origin, helper)
	case OriginLeftover:
		origin.Helper = ""
		origin.Upgrade = docsInstall
		origin.Remove = ""
	default:
		origin.Upgrade = ""
		origin.Remove = ""
	}
}

func applyPackageGuidance(origin *SuiteOrigin, helper string) {
	zsh := helper
	if zsh == "" {
		zsh = filepath.Join("/usr/share/festival/shell", "festival.zsh")
	}
	source := "source " + zsh
	switch origin.Flavor {
	case FlavorAUR:
		origin.Helper = source
		origin.Upgrade = "yay -Syu festival-bin"
		origin.Remove = "yay -Rns festival-bin"
		origin.Package = "festival-bin"
	case FlavorHomebrew:
		origin.Helper = `source "$(brew --prefix)/share/festival/shell/festival.zsh"`
		origin.Upgrade = "brew upgrade --cask festival"
		origin.Remove = "brew uninstall --cask festival"
		origin.Package = "festival"
	case FlavorNpm:
		if helper != "" {
			origin.Helper = "source " + helper
		}
		origin.Upgrade = "npm install -g @obedience-corp/festival@latest"
		origin.Remove = "npm uninstall -g @obedience-corp/festival"
		origin.Package = "@obedience-corp/festival"
	case FlavorDeb:
		origin.Helper = source
		pkg := origin.Package
		if pkg == "" {
			pkg = "obedience-festival"
			origin.Package = pkg
		}
		origin.Upgrade = "reinstall " + pkg + " from " + releasesURL
		origin.Remove = "sudo dpkg -r " + pkg
	case FlavorRpm:
		origin.Helper = source
		pkg := origin.Package
		if pkg == "" {
			pkg = "obedience-festival"
			origin.Package = pkg
		}
		origin.Upgrade = "reinstall " + pkg + " from " + releasesURL
		origin.Remove = "sudo rpm -e " + pkg
	case FlavorApk:
		origin.Helper = source
		pkg := origin.Package
		if pkg == "" {
			pkg = "obedience-festival"
			origin.Package = pkg
		}
		origin.Upgrade = "sudo apk add --allow-untrusted " + pkg + "_*.apk"
		origin.Remove = "sudo apk del " + pkg
	case FlavorInstallSh:
		if helper != "" {
			origin.Helper = "source " + helper
		} else {
			home, _ := os.UserHomeDir()
			origin.Helper = "source " + filepath.Join(home, ".local/share/festival/shell/festival.zsh")
		}
		origin.Upgrade = "curl -fsSL " + installShURL + " | bash"
		origin.Remove = "rm the camp, fest, and festival binaries in " + origin.Prefix
	default:
		if helper != "" {
			origin.Helper = "source " + helper
		}
		origin.Upgrade = releasesURL
		origin.Remove = "see " + releasesURL + " for your distro's remove command"
	}
}
