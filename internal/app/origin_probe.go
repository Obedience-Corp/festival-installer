package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	probeTimeout  = 2 * time.Second
	flavorTimeout = 200 * time.Millisecond
	releasesURL   = "https://github.com/Obedience-Corp/festival/releases/latest"
	docsInstall   = "https://docs.fest.build/getting-started/installation/"
	installShURL  = "https://raw.githubusercontent.com/Obedience-Corp/festival/main/install.sh"
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
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	if tool != selfBinaryName {
		if raw, err := exec.CommandContext(ctx, path, "version", "--json").Output(); err == nil {
			var parsed struct {
				Version string `json:"version"`
				Bundle  string `json:"bundle"`
				Profile string `json:"profile"`
			}
			if json.Unmarshal(raw, &parsed) == nil {
				return parsed.Version, parsed.Bundle, parsed.Profile
			}
		}
	}
	raw, err := exec.CommandContext(ctx, path, "version").Output()
	if err != nil {
		return "", "", ""
	}
	return parseVersionText(tool, string(raw))
}

func parseVersionText(tool, text string) (version, bundle, profile string) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if version == "" {
			switch {
			case strings.HasPrefix(line, "camp "):
				version = strings.TrimSpace(strings.TrimPrefix(line, "camp"))
			case strings.HasPrefix(line, "fest "):
				version = strings.TrimSpace(strings.TrimPrefix(line, "fest"))
			case tool == selfBinaryName:
				version = line
			default:
				version = line
			}
		}
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
