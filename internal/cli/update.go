package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

func NewUpdateCommand() *cobra.Command {
	var channel string
	var asJSON bool
	var allowUnverified bool
	var force bool
	cmd := &cobra.Command{
		Use:   "update [festival|camp|fest]",
		Short: "Update the installed festival suite to the channel-latest release",
		Long: "update brings the installed festival suite (camp + fest) to the channel-latest release.\n\n" +
			"The target argument is optional and defaults to \"festival\", which updates the whole\n" +
			"suite. camp and fest are accepted as aliases: they are not published independently, so\n" +
			"passing either one still updates the whole suite and prints a notice saying so.\n\n" +
			"A package-manager install (AUR, Homebrew, npm, distro packages) is never replaced with\n" +
			"~/.obey/installer. update reports whether a newer suite exists and prints the package\n" +
			"manager upgrade command instead.",
		ValidArgs: []string{"festival", "camp", "fest"},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "festival"
			if len(args) == 1 {
				target = args[0]
			}
			switch target {
			case "festival":
			case "camp", "fest":
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s is part of the festival suite; updating the suite\n", target)
			default:
				return errpkg.New("E_UPDATE_TARGET", "unknown update target "+target+" (expected festival, camp, or fest)")
			}
			if channel != "" {
				if err := app.ValidateChannel(channel); err != nil {
					return err
				}
			}
			vo := source.DefaultVerifyOptions(cmd.ErrOrStderr(), allowUnverified)
			res, warning, err := app.UpdateFestival(cmd.Context(), app.UpdateOptions{
				ChannelOverride: channel,
				Verify:          vo,
				Force:           force,
			})
			if err != nil {
				return err
			}
			var warnings []string
			if warning != "" {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "update: "+warning)
				warnings = []string{warning}
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "update", res, warnings)
			}
			return renderUpdateResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "override the release channel (default: the installed channel)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	cmd.Flags().BoolVar(&allowUnverified, "allow-unverified", false, "allow updating from unsigned content without prompting")
	cmd.Flags().BoolVar(&force, "force", false, "update/install a hub copy even when a package-manager suite is already on PATH")
	return cmd
}

func renderUpdateResult(w io.Writer, res app.UpdateResult) error {
	pkg := textsafe.Line(res.Package)
	switch res.Action {
	case "upgraded":
		if _, err := fmt.Fprintf(w, "upgraded %s %s -> %s\n", pkg, textsafe.Line(res.From), textsafe.Line(res.Version)); err != nil {
			return err
		}
		if res.SelfReplaced {
			_, err := fmt.Fprintf(w, "festival was updated to %s; restart it to use the new version\n", textsafe.Line(res.Version))
			return err
		}
		return nil
	case "current":
		_, err := fmt.Fprintf(w, "%s is already current at %s\n", pkg, textsafe.Line(res.Version))
		return err
	case "unmanaged":
		_, err := fmt.Fprintf(w, "%s is installed outside festival; left untouched\n", pkg)
		return err
	case "package":
		ver := textsafe.Line(res.Version)
		latest := textsafe.Line(res.Latest)
		if latest != "" && ver != "" && installer.VersionLess(ver, latest) {
			_, err := fmt.Fprintf(w, "update available: %s -> %s\nupgrade with the package manager; festival will not plant ~/.obey/installer\n", ver, latest)
			return err
		}
		_, err := fmt.Fprintf(w, "%s is a package-manager install; festival update will not replace it\n", pkg)
		return err
	default:
		_, err := fmt.Fprintf(w, "%s is not installed; run `festival install festival`\n", pkg)
		return err
	}
}
