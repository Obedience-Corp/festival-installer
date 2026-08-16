package cli

import (
	"errors"
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

func NewBrowseCommand() *cobra.Command {
	var asJSON bool
	var product, kind string
	cmd := &cobra.Command{
		Use:   "browse",
		Short: "Browse available packages across registered marketplaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := app.Browse(cmd.Context(), app.BrowseOptions{Product: product, Kind: kind})
			var warning *app.MarketplaceSeedWarning
			if err != nil {
				if !errors.As(err, &warning) {
					return err
				}
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "browse", res, seedWarnings(warning))
			}
			if warning != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warning.Friendly())
			}
			return renderBrowseTable(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	cmd.Flags().StringVar(&product, "product", "", "filter by host product (fest|camp|obey)")
	cmd.Flags().StringVar(&kind, "kind", "", "filter by package class (plugin|tool|product|bundle)")
	return cmd
}

func renderBrowseTable(out io.Writer, res app.BrowseResult) error {
	var buf strings.Builder
	tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "HOST RUNTIME\tID\tNAME\tCLASS\tSOURCE")
	for _, g := range res.Groups {
		for _, p := range g.Packages {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", textsafe.Line(g.HostRuntime), textsafe.Line(p.ID), textsafe.Line(p.DisplayName), textsafe.Line(p.Class), textsafe.Line(p.Source))
		}
	}
	if err := tw.Flush(); err != nil {
		return errpkg.Wrap("E_CLI_RENDER", err, "render browse table")
	}
	_, err := fmt.Fprint(out, buf.String())
	return err
}
