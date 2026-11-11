package app

import (
	"context"
	"os"
	"strings"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

type linuxPlatform struct {
	deb      debPlacer
	appimage appImagePlacer
	hasApt   bool // linuxPackageManager(exists) == "/usr/bin/apt-get"
}

func formatErr(msg string) error { return errpkg.New("E_APP_FORMAT_UNSUPPORTED", msg) }

// chooseLinuxAppFormat applies the D002 and R14 format rules. It never falls
// back between formats silently.
func chooseLinuxAppFormat(requested string, existing *appReceipt, hasApt, debInstalled bool) (string, error) {
	switch requested {
	case "", metadata.ArtifactKindAppImage, metadata.ArtifactKindDeb:
	default:
		return "", formatErr("--format must be appimage or deb")
	}
	if existing != nil && (existing.Format == metadata.ArtifactKindAppImage || existing.Format == metadata.ArtifactKindDeb) {
		if requested == "" || requested == existing.Format {
			return existing.Format, nil
		}
		return "", formatErr("Festival App is installed as " + existing.Format + " at " + existing.Location + ". Run festival uninstall app first to switch formats.")
	}
	switch requested {
	case metadata.ArtifactKindDeb:
		if !hasApt {
			return "", formatErr("--format deb needs apt-get (Debian or Ubuntu family)")
		}
		return metadata.ArtifactKindDeb, nil
	case metadata.ArtifactKindAppImage:
		if debInstalled {
			return "", formatErr("Festival App is installed as a deb package. Remove it with sudo apt-get remove festival-app before installing the AppImage.")
		}
		return metadata.ArtifactKindAppImage, nil
	}
	if hasApt || debInstalled {
		return metadata.ArtifactKindDeb, nil
	}
	return metadata.ArtifactKindAppImage, nil
}

func (p linuxPlatform) format(ctx context.Context, requested string, existing *appReceipt) (string, error) {
	inst, err := p.deb.probe(ctx)
	if err != nil {
		return "", errpkg.Wrap("E_APP_PROBE", err, "ask dpkg about "+debPackage)
	}
	return chooseLinuxAppFormat(requested, existing, p.hasApt, inst.Installed)
}

func (p linuxPlatform) discover(ctx context.Context, format string, existing *appReceipt, rel appRelease) (appCopy, error) {
	if format == metadata.ArtifactKindDeb {
		return p.deb.discover(ctx, existing)
	}
	return p.appimage.discover(ctx, existing, rel)
}

func (p linuxPlatform) target(format string, _ appCopy) (string, error) {
	if format == metadata.ArtifactKindDeb {
		return debLocation, nil
	}
	return p.appimage.target(), nil
}

func (p linuxPlatform) place(ctx context.Context, txn appTxn, req appPlaceRequest) (appPlacement, error) {
	if req.Release.Format == metadata.ArtifactKindDeb {
		return p.deb.place(ctx, txn, req)
	}
	return p.appimage.place(ctx, txn, req)
}

func (p linuxPlatform) onDisk(ctx context.Context, rec appReceipt, latest appLatest) (appCopy, error) {
	if rec.Format == metadata.ArtifactKindDeb {
		inst, err := p.deb.probe(ctx)
		if err != nil {
			return appCopy{Found: true, Location: rec.Location}, err
		}
		if !inst.Installed {
			return appCopy{}, nil
		}
		return appCopy{Found: true, FromReceipt: true, Location: rec.Location, Version: inst.Version, UpdatedByApp: installer.VersionLess(rec.Version, inst.Version)}, nil
	}
	if _, err := os.Lstat(rec.Location); err != nil {
		if os.IsNotExist(err) {
			return appCopy{}, nil
		}
		return appCopy{Found: true, Location: rec.Location}, err
	}
	cp := appCopy{Found: true, FromReceipt: true, Location: rec.Location}
	got, err := artifacts.SHA256(ctx, rec.Location)
	if err != nil {
		return cp, err
	}
	switch {
	case strings.EqualFold(got, rec.ArtifactSHA256):
		cp.Version = rec.Version
	case latest.SHA != "" && strings.EqualFold(got, latest.SHA):
		cp.Version = latest.Version
	default:
		cp.UpdatedByApp = true
	}
	return cp, nil
}
