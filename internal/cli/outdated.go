package cli

import (
	"fmt"
	"io"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
	"github.com/spf13/cobra"
)

func NewOutdatedCommand() *cobra.Command {
	var asJSON, allowUnverified bool
	cmd := &cobra.Command{
		Use: "outdated", Short: "Check installed packages for available updates",
		Long: "Refresh installed sources and check versions without installing or restarting anything.\nReports per-package errors when availability cannot be checked. External installs keep their existing update route.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := app.CheckUpdates(cmd.Context(), source.DefaultVerifyOptions(cmd.ErrOrStderr(), allowUnverified))
			if err != nil {
				return err
			}
			if asJSON {
				return updateEnvelope(cmd.OutOrStdout(), "outdated", result, result.Failed())
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Checked %s\n", result.CheckedAt.Format("2006-01-02 15:04:05 UTC"))
			for _, p := range result.Packages {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  %s -> %s  [%s]\n", textsafe.Line(p.Package), textsafe.Line(p.Installed), textsafe.Line(p.Latest), textsafe.Line(p.Status))
				if p.Detail != "" {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+textsafe.Line(p.Detail))
				}
			}
			if result.Failed() {
				return errpkg.New("E_UPDATE_PARTIAL", "some updates could not be checked; see per-package results and retry")
			}
			if result.Available() > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Run festival update <package> or festival update --all.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	cmd.Flags().BoolVar(&allowUnverified, "allow-unverified", false, "allow checking unsigned content")
	return cmd
}

func runUpdateAll(cmd *cobra.Command, vo source.VerifyOptions, asJSON bool) error {
	result, err := app.UpdateAll(cmd.Context(), app.UpdateOptions{Verify: vo, NoRestart: true})
	if err != nil {
		return err
	}
	if asJSON {
		return updateEnvelope(cmd.OutOrStdout(), "update", result, result.Failed())
	}
	for _, p := range result.Packages {
		if p.Error != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", textsafe.Line(p.Package), textsafe.Line(p.Error))
		} else if p.Action == app.UpdateUnsupported {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: unsupported\n", textsafe.Line(p.Package))
		} else if err := renderUpdateResult(cmd.OutOrStdout(), p.UpdateResult); err != nil {
			return err
		}
		if p.Warning != "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), textsafe.Block(p.Warning))
		}
	}
	if result.Failed() {
		return errpkg.New("E_UPDATE_PARTIAL", "some packages could not be updated; successful updates were retained; retry festival update --all")
	}
	return nil
}

func updateEnvelope(w io.Writer, action string, data any, failed bool) error {
	if !failed {
		return jsonout.Success(w, action, data, nil)
	}
	err := errpkg.New("E_UPDATE_PARTIAL", "some packages could not be checked or updated; see per-package results and retry")
	if writeErr := jsonout.FailureWithData(w, action, errpkg.Code(err), err.Error(), data); writeErr != nil {
		return writeErr
	}
	return jsonAlreadyEmitted(err)
}
