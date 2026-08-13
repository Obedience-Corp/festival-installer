package cli

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

const shortCommitLen = 12

func shortCommit(c string) string {
	if len(c) > shortCommitLen {
		return c[:shortCommitLen]
	}
	return c
}

func NewMarketplaceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "Manage marketplaces",
	}
	cmd.AddCommand(newMarketplaceAddCommand())
	cmd.AddCommand(newMarketplaceRemoveCommand())
	cmd.AddCommand(newMarketplaceListCommand())
	cmd.AddCommand(newMarketplaceRefreshCommand())
	return cmd
}

func newMarketplaceAddCommand() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "add <git-url>",
		Short: "Add a marketplace from a git repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := app.MarketplaceAdd(cmd.Context(), args[0], name)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "added %s (%s) at %s\n", textsafe.Line(src.Name), textsafe.Line(src.URL), textsafe.Line(src.Commit))
			return err
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "override the derived source name")
	return cmd
}

func newMarketplaceRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an added marketplace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.MarketplaceRemove(cmd.Context(), args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", args[0])
			return err
		},
	}
}

func newMarketplaceListCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List added marketplaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			views, err := app.MarketplaceList(cmd.Context())
			if err != nil {
				var warning *app.MarketplaceSeedWarning
				if !errors.As(err, &warning) {
					return err
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", warning)
			}
			if asJSON {
				return jsonout.Print(cmd.OutOrStdout(), views)
			}
			var buf strings.Builder
			tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "NAME\tURL\tCOMMIT\tPACKAGES")
			for _, v := range views {
				pkgs := fmt.Sprintf("%d", v.Packages)
				if v.Err != "" {
					pkgs = "error: " + textsafe.Line(v.Err)
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", textsafe.Line(v.Name), textsafe.Line(v.URL), shortCommit(textsafe.Line(v.Commit)), pkgs)
			}
			if err := tw.Flush(); err != nil {
				return errpkg.Wrap("E_CLI_RENDER", err, "render list table")
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), buf.String())
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}

func newMarketplaceRefreshCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "refresh [name]",
		Short: "Refresh one or all marketplaces",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) == 1 {
				name = args[0]
			}
			views, err := app.MarketplaceRefresh(cmd.Context(), name)
			if err != nil {
				var warning *app.MarketplaceSeedWarning
				if !errors.As(err, &warning) {
					return err
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %v\n", warning)
			}
			if asJSON {
				return jsonout.Print(cmd.OutOrStdout(), views)
			}
			var buf strings.Builder
			for _, v := range views {
				switch {
				case v.Err != "":
					_, _ = fmt.Fprintf(&buf, "%s: error: %s\n", textsafe.Line(v.Name), textsafe.Line(v.Err))
				case v.Changed:
					_, _ = fmt.Fprintf(&buf, "%s: %s -> %s\n", textsafe.Line(v.Name), shortCommit(textsafe.Line(v.OldCommit)), shortCommit(textsafe.Line(v.NewCommit)))
				default:
					_, _ = fmt.Fprintf(&buf, "%s: up to date\n", textsafe.Line(v.Name))
				}
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), buf.String())
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}
