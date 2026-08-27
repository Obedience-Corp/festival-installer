package app

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/release"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

const officialSuiteRepo = "https://github.com/Obedience-Corp/festival.git"

// LookupLatestSuite is the channel-latest suite version from the official
// git repo. Tests replace it. Must not create installer home.
var LookupLatestSuite = lookupLatestSuiteGit

// UpdateOptions configure a suite update.
type UpdateOptions struct {
	ChannelOverride string
	Verify          source.VerifyOptions
	Progress        ProgressFunc
	Force           bool
}

// UpdateFestival upgrades the festival suite to channel-latest when needed.
// Returns a human warning (receipt/live skew or unmanaged guidance) as the second value.
func UpdateFestival(ctx context.Context, opts UpdateOptions) (UpdateResult, string, error) {
	if err := ctx.Err(); err != nil {
		return UpdateResult{}, "", errpkg.Wrap("E_UPDATE_CTX", err, "context cancelled")
	}

	// Resolve self-placement up front so it is reported regardless of which
	// action the update ends up taking.
	selfPlacement, selfPath, err := ResolveSelf(ctx)
	if err != nil {
		return UpdateResult{}, "", err
	}

	// Dual and package origin never plant ~/.obey/installer from update
	// (including --force: tell the user to `festival install --force`).
	origin, _ := DetectSuite(ctx)
	if origin.Kind == OriginPackage || origin.Dual {
		return packageUpdateResult(ctx, opts, origin, selfPlacement, selfPath)
	}

	rec, found, err := ReadFestivalReceipt(ctx)
	if err != nil {
		return UpdateResult{}, "", err
	}
	if !found {
		res, warning, herr := handleUnmanaged(ctx)
		res.SelfPlacement = selfPlacement
		res.SelfPath = selfPath
		return res, warning, herr
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
	if live, derr := detectLiveVersion(ctx, "camp"); derr == nil && LooksLikeVersion(live) && live != installedVersion {
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
		return UpdateResult{Package: FestivalPackageID, Action: "current", Version: installedVersion, SelfPlacement: selfPlacement, SelfPath: selfPath}, warning, nil
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
	selfReplaced := false
	if selfPlacement != SelfManaged {
		note := SelfSkippedNote(selfPlacement, selfPath)
		if warning == "" {
			warning = note
		} else {
			warning = warning + "; " + note
		}
	} else {
		for _, f := range res.Files {
			if filepath.Base(f) == selfBinaryName {
				selfReplaced = true
				break
			}
		}
	}
	return UpdateResult{
		Package:       FestivalPackageID,
		Action:        "upgraded",
		Version:       res.Version,
		From:          installedVersion,
		SelfPlacement: selfPlacement,
		SelfPath:      selfPath,
		SelfReplaced:  selfReplaced,
	}, warning, nil
}

// ReadFestivalReceipt loads the suite receipt if present.
func ReadFestivalReceipt(ctx context.Context) (receipts.Receipt, bool, error) {
	home, err := state.Home(ctx)
	if err != nil {
		return receipts.Receipt{}, false, err
	}
	db, ok, err := state.OpenDBIfExists(ctx, home)
	if err != nil {
		return receipts.Receipt{}, false, err
	}
	if !ok {
		return receipts.Receipt{}, false, nil
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

func packageUpdateResult(ctx context.Context, opts UpdateOptions, origin SuiteOrigin, selfPlacement SelfPlacement, selfPath string) (UpdateResult, string, error) {
	report(opts.Progress, ProgressEvent{Stage: "resolve", Package: FestivalPackageID, Percent: 0.1, Message: "checking for updates"})
	res := UpdateResult{
		Package:       FestivalPackageID,
		Action:        "package",
		Version:       stripVersionPrefix(origin.Version),
		Upgrade:       origin.Upgrade,
		SelfPlacement: selfPlacement,
		SelfPath:      selfPath,
	}
	channel := origin.RelChannel
	if opts.ChannelOverride != "" {
		channel = opts.ChannelOverride
	}
	if channel == "" {
		channel = "stable"
	}
	if latest, err := LookupLatestSuite(ctx, channel); err == nil {
		res.Latest = stripVersionPrefix(latest)
	}
	warning := packageUpdateWarning(res.Version, res.Latest, origin.Upgrade)
	return res, warning, nil
}

func packageUpdateWarning(installed, latest, upgrade string) string {
	var b strings.Builder
	if latest != "" && installed != "" && installer.VersionLess(installed, latest) {
		b.WriteString("update available: ")
		b.WriteString(installed)
		b.WriteString(" -> ")
		b.WriteString(latest)
	} else if latest != "" && installed != "" && !installer.VersionLess(latest, installed) {
		b.WriteString("already current at ")
		b.WriteString(installed)
	}
	if upgrade != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("upgrade with: ")
		b.WriteString(upgrade)
	}
	return b.String()
}

func stripVersionPrefix(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func lookupLatestSuiteGit(ctx context.Context, channel string) (string, error) {
	if channel == "" {
		channel = "stable"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return release.NewResolver().LatestVersion(ctx, officialSuiteRepo, channel)
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

// LooksLikeVersion reports whether s starts with a plausible dotted version
// number, the same check the live-skew probe uses to trust a tool's own
// `version --short` output.
func LooksLikeVersion(s string) bool {
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 3 {
		return false
	}
	return parts[0] != "" && strings.IndexFunc(parts[0], func(r rune) bool { return r < '0' || r > '9' }) == -1
}
