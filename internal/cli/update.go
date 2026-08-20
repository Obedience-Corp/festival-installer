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

func NewUpdateCommand() *cobra.Command {
	var channel string
	var asJSON bool
	var allowUnverified bool
	cmd := &cobra.Command{
		Use:   "update [festival|camp|fest]",
		Short: "Update the installed festival suite to the channel-latest release",
		Long: "update brings the installed festival suite (camp + fest) to the channel-latest release.\n\n" +
			"The target argument is optional and defaults to \"festival\", which updates the whole\n" +
			"suite. camp and fest are accepted as aliases: they are not published independently, so\n" +
			"passing either one still updates the whole suite and prints a notice saying so.",
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
	default:
		_, err := fmt.Fprintf(w, "%s is not installed; run `festival install festival`\n", pkg)
		return err
	}
}
