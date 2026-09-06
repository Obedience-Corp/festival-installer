package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

func NewDoctorCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose installer state (PATH, sources, receipts)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := app.Doctor(cmd.Context())
			failed := app.DoctorFailed(checks)
			if asJSON {
				return emitDoctorJSON(cmd.OutOrStdout(), checks, failed)
			}
			if err := renderDoctorTable(cmd.OutOrStdout(), checks); err != nil {
				return err
			}
			if failed {
				return doctorFailure()
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}

func doctorFailure() error {
	return errpkg.New("E_DOCTOR_FAIL", "one or more doctor checks failed")
}

// emitDoctorJSON writes one envelope whose ok field matches the exit code. The
// failure branch still carries the checks, so a consumer reading a nonzero run
// sees which check failed instead of an error code with no detail.
func emitDoctorJSON(out io.Writer, checks []app.DoctorCheck, failed bool) error {
	data := app.DoctorData{Checks: checks}
	if !failed {
		return jsonout.Success(out, "doctor", data, []string{})
	}
	failErr := doctorFailure()
	if err := jsonout.FailureWithData(out, "doctor", errpkg.Code(failErr), failErr.Error(), data); err != nil {
		return err
	}
	return jsonAlreadyEmitted(failErr)
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
