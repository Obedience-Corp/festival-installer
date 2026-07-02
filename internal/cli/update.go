package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/installer"
	"github.com/Obedience-Corp/obey-installer/internal/jsonout"
	"github.com/Obedience-Corp/obey-installer/internal/source"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

type updateResult struct {
	Package string `json:"package"`
	Action  string `json:"action"`
	Version string `json:"version,omitempty"`
	From    string `json:"from,omitempty"`
}

func NewUpdateCommand() *cobra.Command {
	var channel string
	var asJSON bool
	var allowUnverified bool
	cmd := &cobra.Command{
		Use:   "update <festival|camp|fest>",
		Short: "Update the installed festival suite to the channel-latest release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			switch target {
			case "festival", "camp", "fest":
			default:
				return errpkg.New("E_UPDATE_TARGET", "unknown update target "+target+" (expected festival, camp, or fest)")
			}
			if channel != "" {
				if err := validateChannel(channel); err != nil {
					return err
				}
			}
			vo := source.DefaultVerifyOptions(cmd.ErrOrStderr(), allowUnverified)
			res, warning, err := updateFestival(cmd.Context(), channel, vo)
			if err != nil {
				return err
			}
			if warning != "" {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "update: "+warning)
			}
			if asJSON {
				return jsonout.Success(cmd.OutOrStdout(), "update", res, nil)
			}
			return renderUpdateResult(cmd.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", "override the release channel (default: the installed channel)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON output")
	cmd.Flags().BoolVar(&allowUnverified, "allow-unverified", false, "allow updating from unsigned content without prompting")
	return cmd
}

func updateFestival(ctx context.Context, channelOverride string, vo source.VerifyOptions) (updateResult, string, error) {
	if err := ctx.Err(); err != nil {
		return updateResult{}, "", errpkg.Wrap("E_UPDATE_CTX", err, "context cancelled")
	}

	rec, found, err := readFestivalReceipt(ctx)
	if err != nil {
		return updateResult{}, "", err
	}
	if !found {
		return handleUnmanaged(ctx)
	}

	channel := rec.Channel
	if channelOverride != "" {
		channel = channelOverride
	}
	if channel == "" {
		channel = "stable"
	}

	installedVersion := rec.Version
	warning := ""
	if live, derr := detectLiveVersion(ctx, "camp"); derr == nil && looksLikeVersion(live) && live != installedVersion {
		warning = "receipt reports " + installedVersion + " but the managed camp reports " + live + "; comparing against the live version"
		installedVersion = live
	}

	manifest, err := source.LoadPackageManifest(ctx, rec.Source, festivalPackageID, vo)
	if err != nil {
		return updateResult{}, warning, err
	}
	latest, err := installer.SelectRelease(manifest, channel)
	if err != nil {
		return updateResult{}, warning, err
	}

	if !installer.VersionLess(installedVersion, latest.Version) {
		return updateResult{Package: festivalPackageID, Action: "current", Version: installedVersion}, warning, nil
	}

	res, err := installFestival(ctx, channel, vo)
	if err != nil {
		return updateResult{}, warning, err
	}
	return updateResult{Package: festivalPackageID, Action: "upgraded", Version: res.Version, From: installedVersion}, warning, nil
}

func readFestivalReceipt(ctx context.Context) (receipts.Receipt, bool, error) {
	home, err := state.Home(ctx)
	if err != nil {
		return receipts.Receipt{}, false, err
	}
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		return receipts.Receipt{}, false, err
	}
	defer func() { _ = db.Close(ctx) }()
	rec, err := receipts.Get(ctx, db.Raw(), festivalPackageID)
	if errors.Is(err, receipts.ErrNotFound) {
		return receipts.Receipt{}, false, nil
	}
	if err != nil {
		return receipts.Receipt{}, false, err
	}
	return rec, true, nil
}

func handleUnmanaged(ctx context.Context) (updateResult, string, error) {
	r, err := resolveWhich(ctx, "camp")
	if err != nil {
		return updateResult{Package: festivalPackageID, Action: "absent"},
			"festival is not installed; run `obey-installer install festival`", nil
	}
	loc := r.Path
	if loc == "" {
		loc = r.Managed
	}
	warning := "camp/fest are installed but not managed by obey-installer (found at " + loc + "). " +
		"Refusing to modify an external install. Run `obey-installer which camp --show-all` or `obey-installer doctor` to inspect."
	return updateResult{Package: festivalPackageID, Action: "unmanaged"}, warning, nil
}

func detectLiveVersion(ctx context.Context, tool string) (string, error) {
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return "", err
	}
	out, err := exec.CommandContext(ctx, filepath.Join(binDir, tool), "version", "--short").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func looksLikeVersion(s string) bool {
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 3 {
		return false
	}
	return parts[0] != "" && strings.IndexFunc(parts[0], func(r rune) bool { return r < '0' || r > '9' }) == -1
}

func renderUpdateResult(w io.Writer, res updateResult) error {
	switch res.Action {
	case "upgraded":
		_, err := fmt.Fprintf(w, "upgraded %s %s -> %s\n", res.Package, res.From, res.Version)
		return err
	case "current":
		_, err := fmt.Fprintf(w, "%s is already current at %s\n", res.Package, res.Version)
		return err
	case "unmanaged":
		_, err := fmt.Fprintf(w, "%s is installed outside obey-installer; left untouched\n", res.Package)
		return err
	default:
		_, err := fmt.Fprintf(w, "%s is not installed; run `obey-installer install festival`\n", res.Package)
		return err
	}
}
