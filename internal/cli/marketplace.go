package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/obey-installer/internal/source"
)

func NewMarketplaceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "marketplace",
		Short: "Manage marketplaces",
	}
	cmd.AddCommand(newMarketplaceAddCommand())
	cmd.AddCommand(newMarketplaceRemoveCommand())
	return cmd
}

func newMarketplaceAddCommand() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "add <git-url>",
		Short: "Add a marketplace from a git repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := source.AddMarketplace(cmd.Context(), args[0], name)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "added %s (%s) at %s\n", src.Name, src.URL, src.Commit)
			return err
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "override the derived source name")
	return cmd
}

func newMarketplaceRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an added marketplace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := source.RemoveMarketplace(cmd.Context(), args[0]); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", args[0])
			return err
		},
	}
	return cmd
}
