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

func NewStatusCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report where every suite tool resolves and what version it is",
		Long: "status reports, for camp, fest, festival, obey, and ob: the absolute path the\n" +
			"hub would run, the installer-managed path, the version the binary itself reports,\n" +
			"and the installed package's receipt version and channel. It also reports the\n" +
			"installer home, the managed bin dir, the configured marketplace, and whether the\n" +
			"prerequisites are present.\n\n" +
			"status reports; it does not grade. A missing tool is reported as absent, not as a\n" +
			"failure. Run `festival doctor` for a pass or fail verdict.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rep, err := app.StatusReportFor(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "status", rep, nil)
			}
			return renderStatusTable(cmd.OutOrStdout(), rep)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}

func renderStatusTable(out io.Writer, rep app.StatusReport) error {
	var buf strings.Builder
	_, _ = fmt.Fprintf(&buf, "home:         %s\n", textsafe.Line(rep.Home))
	_, _ = fmt.Fprintf(&buf, "bin dir:      %s\n", textsafe.Line(rep.BinDir))
	_, _ = fmt.Fprintf(&buf, "marketplace:  %s\n\n", textsafe.Line(rep.MarketplaceSource))

	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TOOL\tLOCATION\tVERSION\tORIGIN")
	for _, t := range rep.Tools {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			textsafe.Line(t.Tool),
			textsafe.Line(statusLocation(t)),
			textsafe.Line(statusVersion(t)),
			textsafe.Line(string(t.Origin)))
	}
	if err := tw.Flush(); err != nil {
		return errpkg.Wrap("E_CLI_RENDER", err, "render status table")
	}

	_, _ = fmt.Fprintln(&buf)
	for _, p := range rep.Prerequisites {
		loc := p.Path
		if !p.Present {
			loc = "not found"
		}
		_, _ = fmt.Fprintf(&buf, "prerequisite %s: %s\n", textsafe.Line(p.Name), textsafe.Line(loc))
	}

	_, err := fmt.Fprint(out, buf.String())
	return err
}

func statusLocation(t app.StatusToolEntry) string {
	if t.Path != "" {
		return t.Path
	}
	return "-"
}

func statusVersion(t app.StatusToolEntry) string {
	if t.Version != "" {
		return t.Version
	}
	if t.ReceiptVersion != "" {
		return "(receipt " + t.ReceiptVersion + ")"
	}
	return "-"
}
