package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state"
)

var ErrUnsupportedShell = errpkg.New("E_SHELL_UNSUPPORTED", "unsupported shell")

func NewShellInitCommand() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "shell-init <zsh|bash|fish>",
		Short: "Print shell code to put the installer-managed bin dir on PATH",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			binDir, err := state.BinDir(cmd.Context())
			if err != nil {
				return err
			}
			if check {
				return runPathCheck(cmd.Context(), cmd.OutOrStdout(), binDir)
			}
			snippet, err := shellInitSnippet(args[0], binDir)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), snippet)
			return err
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "report whether the managed bin dir is on PATH")
	return cmd
}

func shellInitSnippet(shell, binDir string) (string, error) {
	switch shell {
	case "zsh", "bash":
		return fmt.Sprintf(`# obey-installer: put managed camp/fest first on PATH
# Add to ~/.zshrc or ~/.bashrc:
#   eval "$(obey-installer shell-init zsh)"
export PATH="%s:$PATH"
`, binDir), nil
	case "fish":
		return fmt.Sprintf(`# obey-installer: put managed camp/fest first on PATH
# Add to ~/.config/fish/config.fish:
#   obey-installer shell-init fish | source
fish_add_path --prepend --global %s
`, binDir), nil
	default:
		return "", errpkg.Wrap("E_SHELL_UNSUPPORTED", ErrUnsupportedShell, shell+" (supported: zsh, bash, fish)")
	}
}

func runPathCheck(ctx context.Context, out io.Writer, binDir string) error {
	if dirOnPath(binDir) {
		if _, err := fmt.Fprintf(out, "managed bin dir is on PATH: %s\n", binDir); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(out, "managed bin dir is NOT on PATH: %s\n", binDir); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "  add it with: eval \"$(obey-installer shell-init zsh)\"\n"); err != nil {
			return err
		}
	}
	for _, tool := range []string{"camp", "fest"} {
		res, err := resolveWhich(ctx, tool)
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
