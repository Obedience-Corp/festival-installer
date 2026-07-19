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
)

func NewWhichCommand() *cobra.Command {
	var asJSON, showAll bool
	cmd := &cobra.Command{
		Use:   "which <tool>",
		Short: "Resolve the real binary path for a suite tool",
		Long: "which resolves where a tool (camp, fest, obey, ...) actually runs from.\n\n" +
			"camp and fest install as shell functions for directory-changing navigation, so a\n" +
			"plain `which camp` prints the function. This resolves the binary on PATH (past the\n" +
			"function) and flags when a binary shadows the installer-managed one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := app.ResolveWhich(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				return jsonout.Print(out, res)
			}
			if showAll {
				return renderWhichTable(out, res)
			}
			loc := res.Path
			if loc == "" {
				loc = res.Managed
			}
			if _, err := fmt.Fprintln(out, loc); err != nil {
				return err
			}
			if res.Shadowed {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: managed %s at %s is shadowed by %s\n", res.Tool, res.Managed, res.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	cmd.Flags().BoolVar(&showAll, "show-all", false, "show the active and managed locations")
	return cmd
}

func renderWhichTable(out io.Writer, res app.WhichResult) error {
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TOOL\tLOCATION\tSOURCE\tSTATUS")
	if res.Path != "" {
		status := "active"
		if res.Shadowed {
			status = "active (shadows managed)"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", res.Tool, res.Path, "path", status)
	}
	if res.Managed != "" && res.Managed != res.Path {
		// Use string compare for display; shadow logic already ran in app.
		status := "managed"
		if res.Shadowed {
			status = "shadowed"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", res.Tool, res.Managed, "managed", status)
	}
	if err := tw.Flush(); err != nil {
		return errpkg.Wrap("E_CLI_RENDER", err, "render which table")
	}
	_, err := fmt.Fprint(out, buf.String())
	return err
}
