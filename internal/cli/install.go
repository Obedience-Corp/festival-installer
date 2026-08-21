package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

func NewInstallCommand() *cobra.Command {
	var channel string
	var asJSON bool
	var allowUnverified bool
	cmd := &cobra.Command{
		Use:   "install <festival|camp|fest>",
		Short: "Install the festival suite (camp, fest, and festival)",
		Long: "install installs the festival suite (camp, fest, and festival).\n\n" +
			"The target is required. festival, camp, and fest all install the suite bundle;\n" +
			"camp and fest are not published independently, so passing either one still installs\n" +
			"the whole suite and prints a notice saying so.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			if err := app.ValidateChannel(channel); err != nil {
				return err
			}
			vo := source.DefaultVerifyOptions(cmd.ErrOrStderr(), allowUnverified)
			if host, _, ok := app.PluginHost(target); !ok {
				switch target {
				case "festival", "camp", "fest":
					if target == "camp" || target == "fest" {
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "installing the festival suite (camp, fest, and festival); camp and fest are not published independently")
					}
				default:
					// InstallTarget will error with a clear message
					_ = host
				}
			}
			res, err := app.InstallTarget(cmd.Context(), target, app.InstallOptions{
				Channel: channel,
				Verify:  vo,
			})
			if err != nil {
				return err
			}
			var warnings []string
			if res.SelfSkipped {
				note := app.SelfSkippedNote(res.SelfPlacement, res.SelfPath)
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "install: "+note)
				warnings = []string{note}
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "install", res, warnings)
			}
			return renderInstallResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "stable", "release channel (stable|rc|dev)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	cmd.Flags().BoolVar(&allowUnverified, "allow-unverified", false, "allow installing unsigned content without prompting")
	return cmd
}

func renderInstallResult(w io.Writer, res app.InstallResult) error {
	if _, err := fmt.Fprintf(w, "installed %s %s (%s) from %s\n", textsafe.Line(res.Package), textsafe.Line(res.Version), textsafe.Line(res.Channel), textsafe.Line(res.Source)); err != nil {
		return err
	}
	for _, f := range res.Files {
		if _, err := fmt.Fprintf(w, "  %s\n", textsafe.Line(f)); err != nil {
			return err
		}
	}
	return nil
}
