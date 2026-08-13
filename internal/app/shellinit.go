package app

import (
	"context"
	"fmt"
	"io"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// ErrUnsupportedShell is returned for unknown shell names.
var ErrUnsupportedShell = errpkg.New("E_SHELL_UNSUPPORTED", "unsupported shell")

// shellSingleQuote wraps s in POSIX single quotes so it is safe to embed in
// bash/zsh/fish snippets even when it contains spaces, quotes, or metacharacters.
// A single quote inside s is encoded as '\” (end quote, escaped quote, reopen).
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ShellInitSnippet returns shell code to prepend the managed bin dir to PATH.
// binDir is shell-quoted so FESTIVAL_HOME / OBEY_INSTALLER_HOME values with
// spaces or shell metacharacters cannot break or inject into the snippet.
func ShellInitSnippet(shell, binDir string) (string, error) {
	quoted := shellSingleQuote(binDir)
	switch shell {
	case "zsh", "bash":
		// Single-quoted bin dir is literal; "$PATH" expands when the rc runs.
		return fmt.Sprintf(`# festival: put managed camp/fest first on PATH
# Add to ~/.zshrc or ~/.bashrc:
#   eval "$(festival shell-init zsh)"
export PATH=%s:"$PATH"
`, quoted), nil
	case "fish":
		return fmt.Sprintf(`# festival: put managed camp/fest first on PATH
# Add to ~/.config/fish/config.fish:
#   festival shell-init fish | source
fish_add_path --prepend --global %s
`, quoted), nil
	default:
		return "", errpkg.Wrap("E_SHELL_UNSUPPORTED", ErrUnsupportedShell, shell+" (supported: zsh, bash, fish)")
	}
}

// ShellInit returns the snippet for the detected managed bin.
func ShellInit(ctx context.Context, shell string) (string, error) {
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return "", err
	}
	return ShellInitSnippet(shell, binDir)
}

// PathCheck writes whether managed bin is on PATH and any shadow warnings.
func PathCheck(ctx context.Context, out io.Writer) error {
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return err
	}
	if dirOnPath(binDir) {
		if _, err := fmt.Fprintf(out, "managed bin dir is on PATH: %s\n", binDir); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(out, "managed bin dir is NOT on PATH: %s\n", binDir); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "  add it with: eval \"$(festival shell-init zsh)\"\n"); err != nil {
			return err
		}
	}
	for _, tool := range []string{"camp", "fest"} {
		res, err := ResolveWhich(ctx, tool)
		if err != nil {
			continue
		}
		if res.Shadowed {
			if _, err := fmt.Fprintf(out, "warning: managed %s at %s is shadowed by %s\n", tool, res.Managed, res.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

// ManagedBinOnPath reports PATH status for the TUI.
func ManagedBinOnPath(ctx context.Context) (binDir string, onPath bool, err error) {
	binDir, err = state.BinDir(ctx)
	if err != nil {
		return "", false, err
	}
	return binDir, dirOnPath(binDir), nil
}
