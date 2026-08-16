package app

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// UpdateOptions configure a suite update.
type UpdateOptions struct {
	ChannelOverride string
	Verify          source.VerifyOptions
	Progress        ProgressFunc
}

// UpdateFestival upgrades the festival suite to channel-latest when needed.
// Returns a human warning (receipt/live skew or unmanaged guidance) as the second value.
func UpdateFestival(ctx context.Context, opts UpdateOptions) (UpdateResult, string, error) {
	if err := ctx.Err(); err != nil {
		return UpdateResult{}, "", errpkg.Wrap("E_UPDATE_CTX", err, "context cancelled")
	}

	rec, found, err := ReadFestivalReceipt(ctx)
	if err != nil {
		return UpdateResult{}, "", err
	}
	if !found {
		return handleUnmanaged(ctx)
	}

	channel := rec.Channel
	if opts.ChannelOverride != "" {
		channel = opts.ChannelOverride
	}
	if channel == "" {
		channel = "stable"
	}
	if err := ValidateChannel(channel); err != nil {
		return UpdateResult{}, "", err
	}

	installedVersion := rec.Version
	warning := ""
	if live, derr := detectLiveVersion(ctx, "camp"); derr == nil && looksLikeVersion(live) && live != installedVersion {
		warning = "receipt reports " + installedVersion + " but the managed camp reports " + live + "; comparing against the live version"
		installedVersion = live
	}

	report(opts.Progress, ProgressEvent{Stage: "resolve", Package: FestivalPackageID, Percent: 0.1, Message: "checking for updates"})

	manifest, err := source.LoadPackageManifest(ctx, rec.Source, FestivalPackageID, opts.Verify)
	if err != nil {
		return UpdateResult{}, warning, err
	}
	latest, err := installer.SelectRelease(manifest, channel)
	if err != nil {
		return UpdateResult{}, warning, err
	}

	if !installer.VersionLess(installedVersion, latest.Version) {
		return UpdateResult{Package: FestivalPackageID, Action: "current", Version: installedVersion}, warning, nil
	}

	res, err := InstallFestival(ctx, InstallOptions{
		Channel:  channel,
		Source:   rec.Source,
		Verify:   opts.Verify,
		Progress: opts.Progress,
	})
	if err != nil {
		return UpdateResult{}, warning, err
	}
	return UpdateResult{Package: FestivalPackageID, Action: "upgraded", Version: res.Version, From: installedVersion}, warning, nil
}

// ReadFestivalReceipt loads the suite receipt if present.
func ReadFestivalReceipt(ctx context.Context) (receipts.Receipt, bool, error) {
	home, err := state.Home(ctx)
	if err != nil {
		return receipts.Receipt{}, false, err
	}
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		return receipts.Receipt{}, false, err
	}
	defer func() { _ = db.Close(ctx) }()
	rec, err := receipts.Get(ctx, db.Raw(), FestivalPackageID)
	if errors.Is(err, receipts.ErrNotFound) {
		return receipts.Receipt{}, false, nil
	}
	if err != nil {
		return receipts.Receipt{}, false, err
	}
	return rec, true, nil
}

func handleUnmanaged(ctx context.Context) (UpdateResult, string, error) {
	r, err := ResolveWhich(ctx, "camp")
	if err != nil {
		return UpdateResult{Package: FestivalPackageID, Action: "absent"},
			"festival is not installed; run `festival install festival`", nil
	}
	loc := r.Path
	if loc == "" {
		loc = r.Managed
	}
	warning := "camp/fest are installed but not managed by festival (found at " + loc + "). " +
		"Refusing to modify an external install. Run `festival which camp --show-all` or `festival doctor` to inspect."
	return UpdateResult{Package: FestivalPackageID, Action: "unmanaged"}, warning, nil
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
