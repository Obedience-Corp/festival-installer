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
		Use:   "uninstall <festival|camp|fest|obey|app|namespace/camp-plugin|namespace/fest-plugin>",
		Short: "Remove an installer-managed package (receipt-owned files only)",
		Long: "uninstall removes receipt-owned files only. app removes Festival App the way it was installed: " +
			"the app bundle on macOS, the App's own uninstaller on Windows, apt-get remove for the deb, and the AppImage " +
			"with its menu entry and icon on Linux. It refuses while Festival App is running.",
		ValidArgs: []string{"festival", "camp", "fest", "obey", "app"},
		Args:      cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.IsAppTarget(args[0]) {
				return runUninstallApp(cmd, asJSON)
			}
			res, warning, err := app.UninstallTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			var warnings []string
			if warning != "" {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "uninstall: "+warning)
				warnings = []string{warning}
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "uninstall", res, warnings)
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

func runUninstallApp(cmd *cobra.Command, asJSON bool) error {
	var out io.Writer
	if !asJSON {
		out = cmd.ErrOrStderr()
	}
	res, err := app.UninstallApp(cmd.Context(), out)
	if err != nil {
		return err
	}
	for _, n := range res.Notes {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "uninstall: "+textsafe.Line(n))
	}
	if asJSON {
		return jsonout.Success(cmd.OutOrStdout(), "uninstall", res, res.Notes)
	}
	return renderAppResult(cmd.OutOrStdout(), "uninstall", res)
}
