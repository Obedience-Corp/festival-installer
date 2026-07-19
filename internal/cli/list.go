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

func NewListCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed packages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := app.ListInstalled(cmd.Context())
			if err != nil {
				return err
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "list", res, nil)
			}
			return renderListTable(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}

func renderListTable(out io.Writer, res app.ListResult) error {
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PACKAGE\tVERSION\tCHANNEL\tSOURCE")
	for _, p := range res.Packages {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.PackageID, p.Version, p.Channel, p.Source)
	}
	if err := tw.Flush(); err != nil {
		return errpkg.Wrap("E_CLI_RENDER", err, "render list table")
	}
	_, err := fmt.Fprint(out, buf.String())
	return err
}
