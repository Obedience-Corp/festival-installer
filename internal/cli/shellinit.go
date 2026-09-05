package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func NewShellInitCommand() *cobra.Command {
	var check bool
	var appendRC bool
	var assumeYes bool
	cmd := &cobra.Command{
		Use:   "shell-init <zsh|bash|fish>",
		Short: "Print shell code to put the installer-managed bin dir on PATH",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if assumeYes && !appendRC {
				return errpkg.New("E_CLI_FLAG", "--yes only applies with --append")
			}
			if check {
				return app.PathCheck(cmd.Context(), cmd.OutOrStdout())
			}
			if appendRC {
				return runShellRCAppend(cmd, args[0], assumeYes)
			}
			snippet, err := app.ShellInit(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), snippet)
			return err
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "report whether the managed bin dir is on PATH")
	cmd.Flags().BoolVar(&appendRC, "append", false, "show the block and offer to append it to your shell rc file")
	cmd.Flags().BoolVar(&assumeYes, "yes", false, "append without asking (requires --append)")
	return cmd
}

// runShellRCAppend shows the exact file and the exact block, then asks. An rc
// file belongs to the user, so this path never writes without a yes on this
// invocation: either --yes, or an answer typed at a real terminal. With neither,
// it prints the block and stops, which leaves a pipe or an agent with something
// to paste instead of an edited rc file nobody approved.
func runShellRCAppend(cmd *cobra.Command, shell string, assumeYes bool) error {
	plan, err := app.PlanShellRCAppend(cmd.Context(), shell)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if _, err := fmt.Fprintf(out, "file:  %s\n\n%s\n", plan.File, plan.Block); err != nil {
		return err
	}
	if plan.Present {
		_, err := fmt.Fprintf(out, "already present in %s; nothing to do\n", plan.File)
		return err
	}
	ok, err := shellRCConsent(cmd, plan.File, assumeYes)
	if err != nil {
		return err
	}
	if !ok {
		_, err := fmt.Fprintf(out, "not appending. Paste the block above into %s when you are ready.\n", plan.File)
		return err
	}
	if err := app.ApplyShellRCAppend(cmd.Context(), plan); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "appended to %s. Restart your shell to pick it up.\n", plan.File)
	return err
}

func shellRCConsent(cmd *cobra.Command, file string, assumeYes bool) (bool, error) {
	if assumeYes {
		return true, nil
	}
	in := cmd.InOrStdin()
	if !readerIsTerminal(in) {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "no terminal to ask on, so nothing was written.")
		return false, err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Append this block to %s? [y/N] ", file); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, errpkg.Wrap("E_CLI_PROMPT", err, "read answer")
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// readerIsTerminal reports whether r is a real terminal. Reading it off the
// command's own input stream rather than os.Stdin keeps the decision testable
// and keeps a redirected stdin from looking interactive.
func readerIsTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
