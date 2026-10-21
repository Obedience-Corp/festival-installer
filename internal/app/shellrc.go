package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// Shell rc guard markers. These match install.sh's markers exactly so the two
// installers never append competing blocks to the same rc file, and so removing
// one block removes it for both.
const (
	shellRCMarkerBegin = "# >>> festival shell integration >>>"
	shellRCMarkerEnd   = "# <<< festival shell integration <<<"
	shellRCNote        = "# Added by festival shell-init. Remove this block to undo it."
)

// ShellRCPlan is the exact PATH persistence change shown before consent. On
// Windows, a managed PowerShell install uses the user Path registry value;
// other shells use a profile block. Planning does not write either target.
type ShellRCPlan struct {
	// Shell is the shell the block was rendered for.
	Shell string
	// File names the profile file or Windows user Path value being changed.
	File string
	// Block shows the profile text or describes the Windows user Path change.
	Block string
	// Present is true when the marker is already in the file, in which case
	// appending would duplicate it.
	Present bool
	// Appendable is false when the block would do nothing once written. A
	// leftover install renders a snippet of pure comments, and writing that
	// under the guard marker would strand the user: the marker is then present,
	// so a later append after the leftover is cleaned up would decline to write
	// the line that actually fixes PATH.
	Appendable bool
	// UserPath selects the Windows user Path value instead of a profile file.
	UserPath bool
	BinDir   string
}

// ShellRCFile returns the rc file festival would append to for shell.
func ShellRCFile(shell string) (string, error) {
	if shell == "powershell" || shell == "pwsh" {
		return powerShellProfile(shell)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errpkg.Wrap("E_SHELL_RC_HOME", err, "cannot resolve $HOME")
	}
	switch shell {
	case "zsh":
		if dir := os.Getenv("ZDOTDIR"); dir != "" {
			if !filepath.IsAbs(dir) {
				return "", errpkg.New("E_SHELL_RC_PATH", "ZDOTDIR must be absolute")
			}
			return filepath.Join(dir, ".zshrc"), nil
		}
		return filepath.Join(home, ".zshrc"), nil
	case "bash":
		return filepath.Join(home, ".bashrc"), nil
	case "fish":
		if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
			if !filepath.IsAbs(dir) {
				return "", errpkg.New("E_SHELL_RC_PATH", "XDG_CONFIG_HOME must be absolute")
			}
			return filepath.Join(dir, "fish", "config.fish"), nil
		}
		return filepath.Join(home, ".config", "fish", "config.fish"), nil
	default:
		return "", errpkg.Wrap("E_SHELL_UNSUPPORTED", ErrUnsupportedShell, shell+" (supported: zsh, bash, fish, powershell, pwsh)")
	}
}

// PlanShellRCAppend computes the PATH persistence change without writing it.
func PlanShellRCAppend(ctx context.Context, shell string) (ShellRCPlan, error) {
	if err := ctx.Err(); err != nil {
		return ShellRCPlan{}, errpkg.Wrap("E_SHELL_RC_CTX", err, "context cancelled before planning rc append")
	}
	guidance, err := ShellGuidanceFor(ctx, shell)
	if err != nil {
		return ShellRCPlan{}, err
	}
	if runtime.GOOS == "windows" && (shell == "powershell" || shell == "pwsh") && guidance.Managed {
		return planWindowsUserPath(ctx, shell, guidance.Bin)
	}
	file, err := ShellRCFile(shell)
	if err != nil {
		return ShellRCPlan{}, err
	}
	snippet := guidance.Snippet
	present, err := shellRCBlockPresent(file)
	if err != nil {
		return ShellRCPlan{}, err
	}
	return ShellRCPlan{
		Shell:      shell,
		File:       file,
		Block:      shellRCBlock(snippet),
		Present:    present,
		Appendable: snippetHasEffect(snippet),
	}, nil
}

// ApplyShellRCAppend applies the approved change. It is idempotent for both
// profile markers and Windows user Path entries.
func ApplyShellRCAppend(ctx context.Context, plan ShellRCPlan) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_SHELL_RC_CTX", err, "context cancelled before rc append")
	}
	if plan.UserPath {
		return applyWindowsUserPath(ctx, plan)
	}
	if plan.File == "" {
		return errpkg.New("E_SHELL_RC_PLAN", "rc append plan has no target file")
	}
	if !plan.Appendable {
		return errpkg.New("E_SHELL_RC_NO_EFFECT", "refusing to write a shell block that would do nothing")
	}
	present, err := shellRCBlockPresent(plan.File)
	if err != nil {
		return err
	}
	if present {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(plan.File), 0o755); err != nil {
		return errpkg.Wrap("E_SHELL_RC_MKDIR", err, "create "+filepath.Dir(plan.File))
	}
	f, err := os.OpenFile(plan.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return errpkg.Wrap("E_SHELL_RC_OPEN", err, "open "+plan.File)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString("\n" + plan.Block); err != nil {
		return errpkg.Wrap("E_SHELL_RC_WRITE", err, "append to "+plan.File)
	}
	return nil
}

// snippetHasEffect reports whether a snippet carries at least one line a shell
// would act on. Checking the content rather than the install origin keeps the
// guard correct for any future snippet that turns out to be advice only.
func snippetHasEffect(snippet string) bool {
	for _, line := range strings.Split(snippet, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return true
	}
	return false
}

func shellRCBlock(snippet string) string {
	var b strings.Builder
	b.WriteString(shellRCMarkerBegin + "\n")
	b.WriteString(shellRCNote + "\n")
	b.WriteString(strings.TrimRight(snippet, "\n") + "\n")
	b.WriteString(shellRCMarkerEnd + "\n")
	return b.String()
}

// shellRCBlockPresent reports whether the guard marker is already in file. A
// missing file is not an error: there is simply no block in it yet.
func shellRCBlockPresent(file string) (bool, error) {
	data, err := os.ReadFile(file)
	switch {
	case os.IsNotExist(err):
		return false, nil
	case err != nil:
		return false, errpkg.Wrap("E_SHELL_RC_READ", err, "read "+file)
	}
	return strings.Contains(string(data), shellRCMarkerBegin), nil
}
