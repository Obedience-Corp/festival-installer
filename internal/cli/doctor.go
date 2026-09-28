package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/doctorui"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
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
			if err := renderDoctor(cmd.OutOrStdout(), checks); err != nil {
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

func renderDoctor(out io.Writer, checks []app.DoctorCheck) error {
	view := doctorui.Render(checks, doctorui.Options{
		Width:   doctorRenderWidth(out),
		Color:   doctorRenderColor(out),
		Heading: true,
	})
	if _, err := io.WriteString(out, view); err != nil {
		return errpkg.Wrap("E_CLI_RENDER", err, "render doctor")
	}
	return nil
}

type fdWriter interface{ Fd() uintptr }

func doctorRenderColor(out io.Writer) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	f, ok := out.(fdWriter)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func doctorRenderWidth(out io.Writer) int {
	f, ok := out.(fdWriter)
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil || width <= 0 {
		return 0
	}
	return width
}
