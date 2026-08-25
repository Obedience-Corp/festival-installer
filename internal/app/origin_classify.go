package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/state"
)

var helperNames = []string{"festival.zsh", "festival.bash", "festival.fish"}

func helperDir(prefix string) string {
	if filepath.Base(prefix) == "bin" {
		return filepath.Join(filepath.Dir(prefix), "share", "festival", "shell")
	}
	return filepath.Join(prefix, "share", "festival", "shell")
}

func helperFile(dir string) string {
	for _, name := range helperNames {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func pathIsUnder(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	path = resolvePath(path)
	root = resolvePath(root)
	if path == root {
		return true
	}
	sep := string(os.PathSeparator)
	prefix := strings.TrimRight(root, sep) + sep
	return strings.HasPrefix(path, prefix)
}

func classifyPath(ctx context.Context, path string) (OriginKind, PackageFlavor, string) {
	if path == "" {
		return OriginLeftover, "", ""
	}
	dir := filepath.Dir(path)

	if exists, err := state.HomeExists(ctx); err == nil && exists {
		if binDir, err := state.BinDir(ctx); err == nil && samePath(dir, binDir) {
			return OriginManaged, "", ""
		}
	}

	if prefix, ok := brewPrefixFor(path); ok {
		if helperFile(filepath.Join(prefix, "share", "festival", "shell")) != "" {
			return OriginPackage, FlavorHomebrew, "festival"
		}
	}

	if flavor, ok := classifyNpm(path); ok {
		return OriginPackage, flavor, "@obedience-corp/festival"
	}

	if h := helperFile(helperDir(dir)); h != "" {
		home, _ := os.UserHomeDir()
		localPrefix := filepath.Join(home, ".local")
		if pathIsUnder(dir, localPrefix) || samePath(dir, filepath.Join(localPrefix, "bin")) {
			return OriginPackage, FlavorInstallSh, ""
		}
		if fl, pkg := flavorFromPackageManager(ctx, path); fl != "" {
			return OriginPackage, fl, pkg
		}
		return OriginPackage, FlavorUnknown, ""
	}

	return OriginLeftover, "", ""
}

func brewPrefixFor(path string) (string, bool) {
	dir := filepath.Dir(path)
	if v := strings.TrimSpace(os.Getenv("HOMEBREW_PREFIX")); v != "" {
		if pathIsUnder(dir, filepath.Join(v, "bin")) || samePath(dir, filepath.Join(v, "bin")) {
			return v, true
		}
	}
	if pathIsUnder(dir, "/opt/homebrew/bin") || samePath(dir, "/opt/homebrew/bin") {
		return "/opt/homebrew", true
	}
	if pathIsUnder(dir, "/home/linuxbrew/.linuxbrew/bin") || samePath(dir, "/home/linuxbrew/.linuxbrew/bin") {
		return "/home/linuxbrew/.linuxbrew", true
	}
	if samePath(dir, "/usr/local/bin") || pathIsUnder(dir, "/usr/local/bin") {
		if runtime.GOOS == "darwin" || fileExists("/usr/local/bin/brew") {
			return "/usr/local", true
		}
	}
	return "", false
}

func classifyNpm(path string) (PackageFlavor, bool) {
	resolved := resolvePath(path)
	dir := filepath.Dir(resolved)
	for i := 0; i < 8 && dir != "" && dir != string(os.PathSeparator); i++ {
		base := filepath.Base(dir)
		helper := helperFile(filepath.Join(dir, "share", "festival", "shell"))
		if helper == "" {
			helper = helperFile(filepath.Join(filepath.Dir(dir), "share", "festival", "shell"))
		}
		if helper != "" && (base == "@obedience-corp" || strings.Contains(dir, filepath.Join("@obedience-corp", "festival"))) {
			return FlavorNpm, true
		}
		if helper != "" && base == "festival" && filepath.Base(filepath.Dir(dir)) == "@obedience-corp" {
			return FlavorNpm, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
