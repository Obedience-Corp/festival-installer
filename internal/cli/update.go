package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/launch"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/textsafe"
)

func NewUpdateCommand() *cobra.Command {
	var channel string
	var asJSON bool
	var allowUnverified bool
	var force bool
	var noRestart bool
	cmd := &cobra.Command{
		Use:   "update [festival|camp|fest|obey]",
		Short: "Update the installed festival suite to the channel-latest release",
		Long: "update brings the installed festival suite (camp + fest) to the channel-latest release.\n\n" +
			"The target argument is optional and defaults to \"festival\", which updates the whole\n" +
			"suite. camp and fest are accepted as aliases: they are not published independently, so\n" +
			"passing either one still updates the whole suite and prints a notice saying so.\n\n" +
			"A package-manager install (AUR, Homebrew, npm) is never replaced with ~/.obey/installer.\n" +
			"When a newer suite exists and stdout is a TTY, update runs the package-manager command\n" +
			"(for example `yay -Syu festival-bin`) so camp, fest, and this hub upgrade together.\n" +
			"--json and non-TTY invocations print the command instead of running it.\n\n" +
			"--no-restart applies to obey only. An obey restart marks every live session failed, so a\n" +
			"caller with running sessions installs the new binaries and defers the restart; the result\n" +
			"reports service.deferred and names the restart command. A daemon that was not running is\n" +
			"started on the new version instead of restarted, and nothing is deferred. The flag is\n" +
			"accepted and ignored for festival, camp, and fest.",
		ValidArgs: []string{"festival", "camp", "fest", "obey"},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "festival"
			if len(args) == 1 {
				target = args[0]
			}
			vo := source.DefaultVerifyOptions(cmd.ErrOrStderr(), allowUnverified)
			switch target {
			case "festival":
			case "camp", "fest":
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s is part of the festival suite; updating the suite\n", target)
			case "obey":
				return runUpdateObey(cmd, channel, vo, asJSON, noRestart)
			default:
				return errpkg.New("E_UPDATE_TARGET", "unknown update target "+target+" (expected festival, camp, fest, or obey)")
			}
			if channel != "" {
				if err := app.ValidateChannel(channel); err != nil {
					return err
				}
			}
			res, warning, err := app.UpdateFestival(cmd.Context(), app.UpdateOptions{
				ChannelOverride: channel,
				Verify:          vo,
				Force:           force,
			})
			if err != nil {
				return err
			}
			var warnings []string
			if warning != "" {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "update: "+warning)
				warnings = []string{warning}
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "update", res, warnings)
			}
			if err := renderUpdateResult(cmd.OutOrStdout(), res); err != nil {
				return err
			}
			return maybeRunPackageUpgrade(cmd, res)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "override the release channel (default: the installed channel)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	cmd.Flags().BoolVar(&allowUnverified, "allow-unverified", false, "allow updating from unsigned content without prompting")
	cmd.Flags().BoolVar(&force, "force", false, "update/install a hub copy even when a package-manager suite is already on PATH")
	cmd.Flags().BoolVar(&noRestart, "no-restart", false,
		"install the new obey binaries without restarting the running daemon")
	return cmd
}

// runUpdateObey is the obey sibling of the suite update body. It never calls
// maybeRunPackageUpgrade: no package manager ships the daemon, so there is no
// package-manager upgrade to hand off to.
func runUpdateObey(cmd *cobra.Command, channel string, vo source.VerifyOptions, asJSON, noRestart bool) error {
	if channel != "" {
		if err := app.ValidateChannel(channel); err != nil {
			return err
		}
	}
	res, warning, err := app.UpdateObey(cmd.Context(), app.UpdateOptions{
		ChannelOverride: channel,
		Verify:          vo,
		NoRestart:       noRestart,
	})
	if err != nil {
		return err
	}
	var warnings []string
	if warning != "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "update: "+warning)
		warnings = []string{warning}
	}
	if asJSON {
		return jsonout.Success(cmd.OutOrStdout(), "update", res, warnings)
	}
	return renderUpdateResult(cmd.OutOrStdout(), res)
}

func renderUpdateResult(w io.Writer, res app.UpdateResult) error {
	pkg := textsafe.Line(res.Package)
	switch res.Action {
	case "upgraded":
		if _, err := fmt.Fprintf(w, "upgraded %s %s -> %s\n", pkg, textsafe.Line(res.From), textsafe.Line(res.Version)); err != nil {
			return err
		}
		if res.SelfReplaced {
			_, err := fmt.Fprintf(w, "festival was updated to %s; restart it to use the new version\n", textsafe.Line(res.Version))
			return err
		}
		return nil
	case "current":
		_, err := fmt.Fprintf(w, "%s is already current at %s\n", pkg, textsafe.Line(res.Version))
		return err
	case "unmanaged":
		_, err := fmt.Fprintf(w, "%s is installed outside festival; left untouched\n", pkg)
		return err
	case "package":
		ver := textsafe.Line(res.Version)
		latest := textsafe.Line(res.Latest)
		if latest != "" && ver != "" && installer.VersionLess(ver, latest) {
			_, err := fmt.Fprintf(w, "update available: %s -> %s\nupgrade with the package manager; festival will not plant ~/.obey/installer\n", ver, latest)
			return err
		}
		_, err := fmt.Fprintf(w, "%s is a package-manager install; festival update will not replace it\n", pkg)
		return err
	default:
		target := "festival"
		if res.Package == app.ObeyPackageID {
			target = "obey"
		}
		_, err := fmt.Fprintf(w, "%s is not installed; run `festival install %s`\n", pkg, target)
		return err
	}
}

func maybeRunPackageUpgrade(cmd *cobra.Command, res app.UpdateResult) error {
	if !app.PackageUpgradeAvailable(res) {
		return nil
	}
	tool, args, ok := app.ParseUpgradeArgv(res.Upgrade)
	if !ok {
		return nil
	}
	if _, err := exec.LookPath(tool); err != nil {
		return nil
	}
	if !cmdStdioIsTTY(cmd) {
		return nil
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "running %s\n", res.Upgrade)
	run := launch.Run(cmd.Context(), launch.Spec{Tool: tool, Args: args, Title: res.Upgrade})
	if run.Err != nil {
		return run.Err
	}
	if run.ExitCode != 0 {
		return errpkg.New("E_UPDATE_PACKAGE", fmt.Sprintf("%s exited %d", res.Upgrade, run.ExitCode))
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), "package upgraded; restart festival to use the new hub")
	return err
}

func cmdStdioIsTTY(cmd *cobra.Command) bool {
	in, inOK := cmd.InOrStdin().(*os.File)
	out, outOK := cmd.OutOrStdout().(*os.File)
	if !inOK || !outOK {
		return false
	}
	return term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}
