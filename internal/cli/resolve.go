package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
)

func NewResolveCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "resolve <tool>",
		Short: "Print the absolute path the hub would run a tool from",
		Long: "resolve prints the absolute path festival itself would run <tool> from: the\n" +
			"installer-managed bin dir first, and PATH only when a package manager owns the\n" +
			"suite.\n\n" +
			"This differs from `festival which`, which reports whatever PATH finds first and\n" +
			"flags shadowing. Use resolve when a program is about to run the binary and which\n" +
			"when a person is asking what is shadowing what.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := app.ResolveTool(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "resolve",
					app.ResolveResult{Tool: args[0], Path: path}, nil)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}
