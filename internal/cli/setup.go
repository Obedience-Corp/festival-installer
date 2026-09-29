package cli

import (
	"fmt"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
	"github.com/spf13/cobra"
)

func NewSetupCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use: "setup", Short: "Create your first festival camp once",
		Long: "Create your starter camp using the installed Camp. Existing camps are preserved. Run this after a package-manager install or to retry starter setup.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := app.SetupStarter(cmd.Context(), "")
			if result.Action == "failed" {
				return errpkg.New("E_STARTER_SETUP", result.Message)
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "setup", result, nil)
			}
			notice := result.Notice()
			if notice == "" {
				notice = "Starter camp setup is already satisfied."
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), textsafe.Block(notice))
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	return cmd
}
