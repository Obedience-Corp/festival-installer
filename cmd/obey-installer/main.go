package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/obey-installer/internal/cli"
)

var version = "0.0.0-dev"

func main() {
	root := &cobra.Command{
		Use:           "obey-installer",
		Short:         "Install and manage obedience corp packages",
		Long:          "obey-installer manages installation and updates of fest, camp, and plugin packages from one or more marketplaces.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	stubs := []struct {
		use, short string
	}{
		{"browse", "Browse available packages"},
		{"list", "List installed packages"},
		{"doctor", "Diagnose installer state"},
	}
	for _, s := range stubs {
		cmd := &cobra.Command{
			Use:   s.use,
			Short: s.short,
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Fprintln(os.Stderr, cmd.Use+": not implemented")
				os.Exit(2)
				return nil
			},
		}
		root.AddCommand(cmd)
	}

	root.AddCommand(cli.NewInstallCommand())
	root.AddCommand(cli.NewUpdateCommand())
	root.AddCommand(cli.NewUninstallCommand())
	root.AddCommand(cli.NewShellInitCommand())
	root.AddCommand(cli.NewMarketplaceCommand())
	root.AddCommand(cli.NewWhichCommand())

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the installer version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
