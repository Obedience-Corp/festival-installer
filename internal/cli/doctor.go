package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/obey-installer/internal/app"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/jsonout"
	"github.com/Obedience-Corp/obey-installer/internal/textsafe"
)

func NewDoctorCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose installer state (PATH, sources, receipts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := app.Doctor(cmd.Context())
			if asJSON {
				if err := jsonout.Success(cmd.OutOrStdout(), "doctor", app.DoctorData{Checks: checks}, []string{}); err != nil {
					return err
				}
			} else if err := renderDoctorTable(cmd.OutOrStdout(), checks); err != nil {
				return err
			}
			if app.DoctorFailed(checks) {
				return errpkg.New("E_DOCTOR_FAIL", "one or more doctor checks failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}

func renderDoctorTable(out io.Writer, checks []app.DoctorCheck) error {
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CHECK\tSTATUS\tDETAIL")
	for _, c := range checks {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", c.ID, c.Status, textsafe.Line(c.Message))
	}
	if err := tw.Flush(); err != nil {
		return errpkg.Wrap("E_CLI_RENDER", err, "render doctor table")
	}
	_, err := fmt.Fprint(out, buf.String())
	return err
}
