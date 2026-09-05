package app

import (
	"context"
	"os"
	"path/filepath"
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

// ShellRCPlan is the exact edit an rc append would make, computed before
// anything is written so the user can be shown the file and the block and then
// asked. Nothing in this type touches the filesystem.
type ShellRCPlan struct {
	// Shell is the shell the block was rendered for.
	Shell string
	// File is the rc file the block would be appended to.
	File string
	// Block is the literal text that would be appended, markers included.
	Block string
	// Present is true when the marker is already in the file, in which case
	// appending would duplicate it.
	Present bool
}

// ShellRCFile returns the rc file festival would append to for shell.
func ShellRCFile(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errpkg.Wrap("E_SHELL_RC_HOME", err, "cannot resolve $HOME")
	}
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc"), nil
	case "bash":
		return filepath.Join(home, ".bashrc"), nil
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish"), nil
	default:
		return "", errpkg.Wrap("E_SHELL_UNSUPPORTED", ErrUnsupportedShell, shell+" (supported: zsh, bash, fish)")
	}
}

// PlanShellRCAppend computes what an append would do without doing it.
func PlanShellRCAppend(ctx context.Context, shell string) (ShellRCPlan, error) {
	if err := ctx.Err(); err != nil {
		return ShellRCPlan{}, errpkg.Wrap("E_SHELL_RC_CTX", err, "context cancelled before planning rc append")
	}
	file, err := ShellRCFile(shell)
	if err != nil {
		return ShellRCPlan{}, err
	}
	snippet, err := ShellInit(ctx, shell)
	if err != nil {
		return ShellRCPlan{}, err
	}
	present, err := shellRCBlockPresent(file)
	if err != nil {
		return ShellRCPlan{}, err
	}
	return ShellRCPlan{
		Shell:   shell,
		File:    file,
		Block:   shellRCBlock(snippet),
		Present: present,
	}, nil
}

// ApplyShellRCAppend appends the planned block. It is a no-op when the marker is
// already present, so running it twice cannot duplicate the block.
func ApplyShellRCAppend(ctx context.Context, plan ShellRCPlan) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_SHELL_RC_CTX", err, "context cancelled before rc append")
	}
	if plan.File == "" {
		return errpkg.New("E_SHELL_RC_PLAN", "rc append plan has no target file")
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
