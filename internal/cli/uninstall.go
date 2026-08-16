package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

func NewUninstallCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "uninstall <festival|camp|fest>",
		Short: "Remove the installer-managed festival suite (receipt-owned files only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := app.UninstallTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "uninstall", res, nil)
			}
			return renderUninstallResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}

func renderUninstallResult(w io.Writer, res app.UninstallResult) error {
	if res.Note != "" {
		_, err := fmt.Fprintln(w, res.Note)
		return err
	}
	if _, err := fmt.Fprintf(w, "uninstalled %s\n", textsafe.Line(res.Package)); err != nil {
		return err
	}
	for _, f := range res.Removed {
		if _, err := fmt.Fprintf(w, "  removed %s\n", textsafe.Line(f)); err != nil {
			return err
		}
	}
	return nil
}
