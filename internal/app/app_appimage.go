package app

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/installer"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

const (
	appImageName    = "festival-app"
	appImageIconRel = "usr/share/icons/hicolor/512x512/apps/festival-app.png" // confirmed on the real 0.1.7 AppImage
	appDesktopMark  = "X-Festival-Installer-Package=obedience-corp/festival-app"
	appIconNote     = "Could not extract the Festival App icon; the menu entry uses a generic icon."
)

type appImagePlacer struct {
	run       appCommandRunner
	binDir    string // $HOME/.local/bin
	dataHome  string // xdgDataHome(os.Getenv, home)
	stageRoot string // $FESTIVAL_HOME: --appimage-extract runs in a private dir here
}

// xdgDataHome is $XDG_DATA_HOME when set and absolute, else home/.local/share.
func xdgDataHome(getenv func(string) string, home string) string {
	if v := getenv("XDG_DATA_HOME"); v != "" && filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(home, ".local", "share")
}

func (a appImagePlacer) target() string { return filepath.Join(a.binDir, appImageName) }
func (a appImagePlacer) desktopPath() string {
	return filepath.Join(a.dataHome, "applications", "festival-app.desktop")
}
func (a appImagePlacer) iconPath() string {
	return filepath.Join(a.dataHome, "icons", "hicolor", "512x512", "apps", "festival-app.png")
}

// desktopExecQuote quotes path for the Exec key per the Desktop Entry spec.
func desktopExecQuote(path string) (string, error) {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range path {
		switch {
		case r < 0x20 || r == 0x7f:
			return "", errpkg.New("E_APP_INSTALLER_FAILED", "the install path contains a control character")
		case r == '"' || r == '`' || r == '$':
			b.WriteString(`\\`)
			b.WriteRune(r)
		case r == '\\':
			b.WriteString(`\\\\`)
		case r == '%':
			b.WriteString("%%")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

func desktopEntry(exe string) (string, error) {
	quoted, err := desktopExecQuote(exe)
	if err != nil {
		return "", err
	}
	return "[Desktop Entry]\nType=Application\nName=Festival App\nExec=" + quoted + "\nTryExec=" + exe +
		"\nIcon=festival-app\nStartupWMClass=festival-app\nTerminal=false\nCategories=Development;\n" + appDesktopMark + "\n", nil
}

func (a appImagePlacer) discover(ctx context.Context, existing *appReceipt, rel appRelease) (appCopy, error) {
	hasReceipt := existing != nil && existing.Format == metadata.ArtifactKindAppImage
	if hasReceipt && existing.Location != a.target() {
		return appCopy{}, foreignAppError(existing.Location, "festival manages only "+a.target())
	}
	fi, err := os.Lstat(a.target())
	if os.IsNotExist(err) {
		return appCopy{}, nil
	}
	if err != nil {
		return appCopy{}, errpkg.Wrap("E_APP_PROBE", err, "inspect "+a.target())
	}
	if !fi.Mode().IsRegular() {
		return appCopy{}, foreignAppError(a.target(), "not a regular file")
	}
	if !hasReceipt {
		return appCopy{}, foreignAppError(a.target(), "no festival record for it")
	}
	cp := appCopy{Found: true, FromReceipt: true, Location: a.target()}
	_, matches, err := appImageMatchesReceipt(ctx, a.target(), existing.ArtifactSHA256)
	if err != nil {
		return appCopy{}, err
	}
	if matches {
		cp.Version = existing.Version
		return cp, nil
	}
	got, err := artifacts.SHA256(ctx, a.target())
	if err != nil {
		return appCopy{}, err
	}
	if strings.EqualFold(got, rel.Artifact.Sha256) {
		cp.Version = rel.Version
		return cp, nil
	}
	cp.UpdatedByApp = true
	return cp, nil
}

// appImageExtras tracks the menu entry files one place call touched.
type appImageExtras struct {
	temps                       []string
	iconCreated, desktopCreated bool
	iconPlaced, desktopPlaced   bool
}

func (a appImagePlacer) cleanupAfterFailure(x *appImageExtras) {
	if x.iconPlaced && x.iconCreated {
		_ = os.Remove(a.iconPath())
	}
	if x.desktopPlaced && x.desktopCreated {
		_ = os.Remove(a.desktopPath())
	}
	for _, t := range x.temps {
		_ = os.Remove(t)
	}
}

func siblingTemp(path string) string {
	return filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".festival-new-"+fmt.Sprintf("%d", time.Now().UnixNano()))
}

func writeSibling(final string, mode os.FileMode, write func(w io.Writer) error) (string, error) {
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return "", err
	}
	tmp := siblingTemp(final)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return "", err
	}
	if err := write(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return tmp, os.Chmod(tmp, mode)
}

func copyInto(dst string, mode os.FileMode, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

// extractIcon runs the AppImage's own --appimage-extract in a private dir and
// returns the icon path inside it. The caller removes the dir.
func (a appImagePlacer) extractIcon(ctx context.Context, payload string) (dir, icon string, err error) {
	dir, err = os.MkdirTemp(a.stageRoot, "appimage-extract-")
	if err != nil {
		return "", "", err
	}
	res, err := a.run(ctx, 2*time.Minute, dir, nil, payload, "--appimage-extract", appImageIconRel)
	if err != nil {
		return dir, "", err
	}
	if res.ExitCode != 0 {
		return dir, "", fmt.Errorf("--appimage-extract exited %d", res.ExitCode)
	}
	icon = filepath.Join(dir, "squashfs-root", filepath.FromSlash(appImageIconRel))
	fi, err := os.Lstat(icon)
	if err != nil || !fi.Mode().IsRegular() {
		return dir, "", fmt.Errorf("no icon at %s", appImageIconRel)
	}
	return dir, icon, nil
}

func (a appImagePlacer) place(ctx context.Context, txn appTxn, req appPlaceRequest) (appPlacement, error) {
	rel := req.Release
	if err := os.MkdirAll(a.binDir, 0o755); err != nil {
		return appPlacement{}, errpkg.Wrap("E_INSTALL_STAGING", err, "create "+a.binDir)
	}
	payload, err := txn.Prepare(ctx, installer.AppPlacement{PackageID: AppPackageID, Version: rel.Version, Format: metadata.ArtifactKindAppImage, ArtifactSHA256: rel.Artifact.Sha256, Dest: a.target()})
	if err != nil {
		return appPlacement{}, err
	}
	x := &appImageExtras{}
	ok := false
	defer func() {
		if !ok {
			a.cleanupAfterFailure(x)
		}
	}()
	if err := copyInto(payload, 0o755, req.Artifact.Path); err != nil {
		return appPlacement{}, errpkg.Wrap("E_INSTALL_STAGING", err, "copy the AppImage into place")
	}

	var notes []string
	iconTmp := ""
	if dir, icon, err := a.extractIcon(ctx, payload); err != nil {
		notes = append(notes, appIconNote)
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
	} else {
		iconTmp, err = writeSibling(a.iconPath(), 0o644, func(w io.Writer) error {
			in, e := os.Open(icon)
			if e != nil {
				return e
			}
			defer func() { _ = in.Close() }()
			_, e = io.Copy(w, in)
			return e
		})
		_ = os.RemoveAll(dir)
		if err != nil {
			notes = append(notes, appIconNote)
			iconTmp = ""
		} else {
			x.temps = append(x.temps, iconTmp)
		}
	}

	entry, err := desktopEntry(a.target())
	if err != nil {
		return appPlacement{}, err
	}
	desktopTmp := ""
	if raw, readErr := os.ReadFile(a.desktopPath()); readErr == nil && !strings.Contains(string(raw), appDesktopMark) {
		notes = append(notes, "Left your existing "+a.desktopPath()+" unchanged.")
	} else {
		x.desktopCreated = readErr != nil
		desktopTmp, err = writeSibling(a.desktopPath(), 0o644, func(w io.Writer) error { _, e := io.WriteString(w, entry); return e })
		if err != nil {
			return appPlacement{}, errpkg.Wrap("E_INSTALL_STAGING", err, "write the desktop entry")
		}
		x.temps = append(x.temps, desktopTmp)
	}

	// Menu entry first, so there is never an installed App without it.
	if iconTmp != "" {
		_, statErr := os.Lstat(a.iconPath())
		x.iconCreated = statErr != nil
		if err := os.Rename(iconTmp, a.iconPath()); err != nil {
			return appPlacement{}, errpkg.Wrap("E_INSTALL_STAGING", err, "place the icon")
		}
		x.iconPlaced = true
	}
	if desktopTmp != "" {
		if err := os.Rename(desktopTmp, a.desktopPath()); err != nil {
			return appPlacement{}, errpkg.Wrap("E_INSTALL_STAGING", err, "place the desktop entry")
		}
		x.desktopPlaced = true
	}

	if err := txn.MarkStaged(ctx); err != nil {
		return appPlacement{}, err
	}
	if err := txn.Swap(ctx); err != nil {
		return appPlacement{}, err
	}
	exe, err := appMainExecutable(metadata.ArtifactKindAppImage, a.target())
	if err != nil {
		return appPlacement{}, err
	}
	placement := appPlacement{Location: a.target(), Executable: exe, Mode: 0o755, Notes: notes}
	leftover, err := txn.Commit(ctx, func(ctx context.Context, db *sql.DB) error { return req.WriteReceipt(ctx, db, placement) })
	if err != nil {
		return appPlacement{}, err
	}
	ok = true
	if leftover != "" {
		placement.Notes = append(placement.Notes, "Could not remove the previous copy at "+leftover+"; festival retries on its next run.")
	}
	return placement, nil
}

func (a appImagePlacer) remove(_ context.Context, rec appReceipt, _ io.Writer) (bool, []string, error) {
	if rec.Location != a.target() {
		return false, nil, foreignAppError(rec.Location, "festival manages only "+a.target())
	}
	var notes []string
	removed := false
	fi, err := os.Lstat(a.target())
	switch {
	case os.IsNotExist(err):
		notes = append(notes, removedOutsideNote2)
	case err != nil:
		return false, nil, errpkg.Wrap("E_APP_PROBE", err, "inspect "+a.target())
	case !fi.Mode().IsRegular():
		return false, nil, foreignAppError(a.target(), "not a regular file")
	default:
		if err := os.Remove(a.target()); err != nil {
			return false, nil, errpkg.Wrap("E_INSTALL_STAGING", err, "remove "+a.target())
		}
		removed = true
	}
	if raw, err := os.ReadFile(a.desktopPath()); err == nil {
		if strings.Contains(string(raw), appDesktopMark) {
			_ = os.Remove(a.desktopPath())
		} else {
			notes = append(notes, "Left your "+a.desktopPath()+" because festival did not write it.")
		}
	}
	if fi, err := os.Lstat(a.iconPath()); err == nil && fi.Mode().IsRegular() {
		_ = os.Remove(a.iconPath())
	}
	return removed, notes, nil
}
