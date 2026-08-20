package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewVersionCommand prints the festival hub's own version. version is the
// build-time value main injects via -X main.version=....
func NewVersionCommand(version string) *cobra.Command {
	var short bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the festival manager version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), version)
			return err
		},
	}
	cmd.Flags().BoolVar(&short, "short", false, "print only the version number (the default output already is)")
	return cmd
}
