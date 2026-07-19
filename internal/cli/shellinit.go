package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/obey-installer/internal/app"
)

func NewShellInitCommand() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "shell-init <zsh|bash|fish>",
		Short: "Print shell code to put the installer-managed bin dir on PATH",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if check {
				return app.PathCheck(cmd.Context(), cmd.OutOrStdout())
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
	return cmd
}
