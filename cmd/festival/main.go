package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	// bginit must load before bubbletea (via tui) so lipgloss skips OSC queries.
	_ "github.com/Obedience-Corp/obey-installer/internal/bginit"
	"github.com/Obedience-Corp/obey-installer/internal/cli"
	"github.com/Obedience-Corp/obey-installer/internal/tui"
)

var version = "0.0.0-dev"

func main() {
	var forceTUI bool
	root := &cobra.Command{
		Use:   "festival",
		Short: "Install and manage Festival tools and plugins",
		Long: `festival is the Festival manager: install, update, and browse camp, fest,
and plugins. Run with no arguments to open the interactive TUI.

A festival has a lot of different things going on, just like you.

Scripts and agents can use subcommands with --json for machine-readable output.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			launchTUI, err := tuiLaunchDecision(
				forceTUI,
				term.IsTerminal(int(os.Stdin.Fd())),
				term.IsTerminal(int(os.Stdout.Fd())),
			)
			if err != nil {
				return err
			}
			if launchTUI {
				return tui.Run(cmd.Context(), tui.Options{Version: version})
			}
			_, _ = fmt.Fprint(cmd.ErrOrStderr(), nonTTYBareMessage())
			return cmd.Help()
		},
	}
	root.Flags().BoolVar(&forceTUI, "tui", false, "force the interactive TUI (requires a terminal)")

	root.AddCommand(cli.NewInstallCommand())
	root.AddCommand(cli.NewUpdateCommand())
	root.AddCommand(cli.NewUninstallCommand())
	root.AddCommand(cli.NewListCommand())
	root.AddCommand(cli.NewShellInitCommand())
	root.AddCommand(cli.NewBrowseCommand())
	root.AddCommand(cli.NewDoctorCommand())
	root.AddCommand(cli.NewMarketplaceCommand())
	root.AddCommand(cli.NewWhichCommand())

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the festival manager version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	})

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func tuiLaunchDecision(force, stdinTTY, stdoutTTY bool) (bool, error) {
	if force && (!stdinTTY || !stdoutTTY) {
		return false, fmt.Errorf("TUI requires an interactive terminal; use festival <command> or open a TTY")
	}
	return force || (stdinTTY && stdoutTTY), nil
}

func nonTTYBareMessage() string {
	return `festival: no interactive terminal detected.

Open a terminal and run:  festival
Or use a subcommand:      festival install festival
                          festival --help
`
}
