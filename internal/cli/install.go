package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

func NewInstallCommand() *cobra.Command {
	var channel string
	var asJSON bool
	var allowUnverified bool
	var force bool
	var noRestart bool
	var noIntegrations bool
	var format string
	cmd := &cobra.Command{
		Use:   "install <festival|camp|fest|obey|app|namespace/camp-plugin|namespace/fest-plugin>",
		Short: "Install the festival suite (camp, fest, and festival)",
		Long: "install installs the festival suite (camp, fest, and festival).\n\n" +
			"The target is required. festival, camp, and fest all install the suite bundle;\n" +
			"camp and fest are not published independently, so passing either one still installs\n" +
			"the whole suite and prints a notice saying so.\n\n" +
			"obey installs the obey daemon and the ob developer CLI as their own package.\n\n" +
			"A namespace/camp-* or namespace/fest-* target selects that exact marketplace plugin.\n\n" +
			"--no-restart applies to obey only. Installing over a running daemon restarts it so the\n" +
			"supervised process is the version that was just placed, which marks every live session\n" +
			"failed; --no-restart installs the binaries, leaves the daemon alone, and reports the\n" +
			"restart as pending. The flag is accepted and ignored for festival, camp, and fest.\n\n" +
			"app installs Festival App, the desktop app, the standard way for this system: the app bundle in /Applications (or ~/Applications) on macOS, " +
			"the App's own installer on Windows, the deb through apt on Debian and Ubuntu, and the AppImage in ~/.local/bin elsewhere. " +
			"Every download is checked against the signed marketplace and the App's own update signature before anything is placed. " +
			"--format appimage or --format deb chooses the Linux format. Quit Festival App before installing over it.",
		ValidArgs: []string{"festival", "camp", "fest", "obey", "app"},
		Args:      cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			vo := source.DefaultVerifyOptions(cmd.ErrOrStderr(), allowUnverified)
			if app.IsAppTarget(target) {
				return runInstallApp(cmd, format, channel, vo, asJSON, force)
			}
			if format != "" {
				return errpkg.New("E_APP_FORMAT_UNSUPPORTED", "--format applies only to festival install app")
			}
			if err := app.ValidateChannel(channel); err != nil {
				return err
			}
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
				Channel:        channel,
				Verify:         vo,
				Force:          force,
				NoRestart:      noRestart,
				NoIntegrations: noIntegrations,
			})
			if err != nil {
				return err
			}
			var warnings []string
			if note := app.IntegrationNote(res.Integrations); note != "" {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "install: "+textsafe.Block(note))
				if !res.Integrations.Ready {
					warnings = append(warnings, note)
				}
			}
			if res.SelfSkipped {
				note := app.SelfSkippedNote(res.SelfPlacement, res.SelfPath)
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "install: "+note)
				warnings = append(warnings, note)
			}
			if note := app.ServiceNoteFor(res); note != "" {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "install: "+note)
				warnings = append(warnings, note)
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
	cmd.Flags().BoolVar(&force, "force", false, "install a hub copy even when a package-manager suite is already on PATH")
	cmd.Flags().BoolVar(&noRestart, "no-restart", false,
		"install the new obey binaries without restarting the running daemon")
	cmd.Flags().StringVar(&format, "format", "", "Linux only: install Festival App as appimage or deb")
	cmd.Flags().BoolVar(&noIntegrations, "no-shell-setup", false, "disable automatic terminal integration (remembered across updates)")
	return cmd
}

func renderInstallResult(w io.Writer, res app.InstallResult) error {
	if notice := res.Starter.Notice(); notice != "" {
		if _, err := fmt.Fprintln(w, textsafe.Block(notice)); err != nil {
			return err
		}
	}
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

func runInstallApp(cmd *cobra.Command, format, channel string, vo source.VerifyOptions, asJSON, force bool) error {
	if format != "" && format != "appimage" && format != "deb" {
		return errpkg.New("E_APP_FORMAT_UNSUPPORTED", "--format must be appimage or deb")
	}
	if channel != "" && channel != "stable" {
		return errpkg.New("E_APP_NOT_AVAILABLE", "Festival App is published on the stable channel only")
	}
	var warnings []string
	if force {
		warnings = append(warnings, "--force has no effect for the App")
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "install: --force has no effect for the App")
	}
	opts := app.AppInstallOptions{Format: format, Verify: vo}
	if !asJSON {
		opts.Progress = app.NewTextProgress(cmd.ErrOrStderr(), cmdStdioIsTTY(cmd))
		opts.Output = cmd.ErrOrStderr()
	}
	res, err := app.InstallApp(cmd.Context(), opts)
	if err != nil {
		return err
	}
	if asJSON {
		for _, n := range res.Notes {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "install: "+textsafe.Line(n))
		}
		return jsonout.Success(cmd.OutOrStdout(), "install", res, append(warnings, res.Notes...))
	}
	return renderAppResult(cmd.OutOrStdout(), "install", res)
}
