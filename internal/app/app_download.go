package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

const (
	appDownloadsDir  = "app-downloads"
	appStallTimeout  = 60 * time.Second
	appDownloadStale = 24 * time.Hour
)

type appDownloadOptions struct {
	UpdaterKey   *verify.MinisignPublicKey // nil: verify.FestivalAppUpdaterKey()
	StallTimeout time.Duration             // 0: appStallTimeout
	MaxBytes     int64                     // 0: the artifacts default (1 GiB)
	Progress     ProgressFunc
}

type verifiedAppArtifact struct {
	Path    string // the verified file
	Dir     string // $FESTIVAL_HOME/app-downloads/run-*; remove with Cleanup
	Release appRelease
}

// Cleanup removes the run directory holding the verified file.
func (v verifiedAppArtifact) Cleanup() {
	if v.Dir != "" {
		_ = os.RemoveAll(v.Dir)
	}
}

// downloadAppArtifact downloads and verifies one App artifact. It never takes
// the installer lock; callers take the lock only after it returns.
func downloadAppArtifact(ctx context.Context, home string, rel appRelease, opts appDownloadOptions) (verifiedAppArtifact, error) {
	if err := ctx.Err(); err != nil {
		return verifiedAppArtifact{}, errpkg.Wrap("E_APP_DOWNLOAD_CTX", err, "context cancelled before download")
	}
	if strings.TrimSpace(rel.Artifact.UpdaterSignature) == "" {
		return verifiedAppArtifact{}, errpkg.New("E_APP_SIGNATURE", "no updater signature for "+rel.Artifact.Filename)
	}
	if err := state.EnsureHome(ctx, 0o700); err != nil {
		return verifiedAppArtifact{}, err
	}
	root := filepath.Join(home, appDownloadsDir)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return verifiedAppArtifact{}, errpkg.Wrap("E_APP_DOWNLOAD_DIR", err, "create "+root)
	}
	pruneStaleAppDownloads(root, time.Now())
	dir, err := os.MkdirTemp(root, "run-")
	if err != nil {
		return verifiedAppArtifact{}, errpkg.Wrap("E_APP_DOWNLOAD_DIR", err, "create download run directory")
	}
	fail := func(err error) (verifiedAppArtifact, error) {
		_ = os.RemoveAll(dir)
		return verifiedAppArtifact{}, err
	}

	stall := opts.StallTimeout
	if stall <= 0 {
		stall = appStallTimeout
	}
	message := "Downloading Festival App " + rel.Version
	d := artifacts.NewStallDownloader(stall, func(written, total int64) {
		percent := -1.0
		if total > 0 {
			percent = float64(written) / float64(total)
		}
		report(opts.Progress, ProgressEvent{Stage: "download", Package: AppPackageID, Bytes: written, Percent: percent, Message: message})
	})
	if opts.MaxBytes > 0 {
		d.MaxBytes = opts.MaxBytes
	}
	path, err := d.Download(ctx, rel.Artifact.URL, dir)
	if err != nil {
		if errpkg.Code(err) == "E_ARTIFACT_STALLED" {
			return fail(errpkg.Wrap("E_APP_DOWNLOAD_STALLED", err, "the Festival App download stopped receiving data for "+stall.String()+"; check the network and run the command again"))
		}
		return fail(err)
	}

	report(opts.Progress, ProgressEvent{Stage: "verify", Package: AppPackageID, Percent: -1, Message: "Verifying Festival App " + rel.Version})
	if err := artifacts.VerifySHA256(ctx, path, rel.Artifact.Sha256); err != nil {
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		return fail(errpkg.Wrap("E_APP_SIGNATURE", err, "the Festival App download does not match the marketplace checksum"))
	}
	key := verify.FestivalAppUpdaterKey()
	if opts.UpdaterKey != nil {
		key = *opts.UpdaterKey
	}
	if err := verify.VerifyTauriSignatureFile(ctx, key, rel.Artifact.UpdaterSignature, path); err != nil {
		if ctx.Err() != nil {
			return fail(ctx.Err())
		}
		return fail(errpkg.Wrap("E_APP_SIGNATURE", err, "the Festival App updater signature does not verify"))
	}
	return verifiedAppArtifact{Path: path, Dir: dir, Release: rel}, nil
}

// pruneStaleAppDownloads removes run directories left by a killed process. A
// run that is still downloading is never older than appDownloadStale.
func pruneStaleAppDownloads(root string, now time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "run-") {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) <= appDownloadStale {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, e.Name()))
	}
}
