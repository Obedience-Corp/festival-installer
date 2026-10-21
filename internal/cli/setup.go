package cli

import (
	"context"
	"fmt"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
	"github.com/spf13/cobra"
)

func NewSetupCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use: "setup", Short: "Create your first festival camp once",
		Long: "Create your starter camp using the installed Camp. Existing camps are preserved. Run this after a package-manager install or to retry starter setup.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := app.SetupStarter(cmd.Context(), "")
			if result.Action == "failed" {
				return errpkg.New("E_STARTER_SETUP", result.Message)
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "setup", result, nil)
			}
			notice := result.Notice()
			if notice == "" {
				notice = "Starter camp setup is already satisfied."
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), textsafe.Block(notice))
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	cmd.AddCommand(newIntegrationsCommand())
	cmd.AddCommand(newPrerequisitesCommand(app.SetupPrerequisites))
	return cmd
}

func newPrerequisitesCommand(run func(context.Context) app.PrerequisiteReport) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use: "prerequisites", Short: "Install missing Git using the system installer",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report := run(cmd.Context())
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "setup prerequisites", report, nil)
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), textsafe.Block(report.Message)); err != nil {
				return err
			}
			if !report.Ready {
				return errpkg.New("E_PREREQUISITES_SETUP", "Git is not ready; see the setup message above.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit a structured prerequisite report")
	return cmd
}

func newIntegrationsCommand() *cobra.Command {
	var opts app.IntegrationOptions
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "integrations",
		Short: "Configure or repair terminal helpers and Obey provider hooks",
		Long:  "Configure camp/fest shell helpers and Obey wrapping, preserving unrelated settings. Existing terminals need reopening. Use --check for read-only inspection or --disable to remove owned startup blocks and remember the opt-out.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := app.ReconcileIntegrations(cmd.Context(), opts)
			if err != nil {
				return err
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "setup integrations", report, nil)
			}
			for _, check := range report.Checks {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", textsafe.Line(check.Status), textsafe.Line(check.Message)); err != nil {
					return err
				}
			}
			if report.Ready {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Terminal setup is ready. Open a new terminal to use newly configured helpers.")
				return err
			}
			// An explicit, successfully applied opt-out is a completed request.
			if opts.Disable && report.Disabled {
				for _, check := range report.Checks {
					// Hook repair is independent of removing terminal startup
					// blocks. Keep reporting it, but do not fail the opt-out.
					if check.ID == "bridge" {
						continue
					}
					if check.Status != "ready" && check.Status != "not_required" && check.Status != "disabled" {
						return errpkg.New("E_INTEGRATION_SETUP", "Terminal setup needs attention; see the checks above.")
					}
				}
				return nil
			}
			return errpkg.New("E_INTEGRATION_SETUP", "Terminal setup is not ready; see the checks above.")
		},
	}
	cmd.Flags().StringVar(&opts.Shell, "shell", "", "shell name or absolute executable (defaults to login shell; both installed PowerShell editions on Windows)")
	cmd.Flags().BoolVar(&opts.Check, "check", false, "inspect without changing configuration")
	cmd.Flags().BoolVar(&opts.Disable, "disable", false, "remove owned startup blocks and remember the opt-out")
	cmd.Flags().BoolVar(&opts.Enable, "enable", false, "enable automatic integration after an opt-out")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit a structured readiness report")
	return cmd
}
