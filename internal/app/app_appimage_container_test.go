//go:build container_fs && linux

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

const fakeAppImageScript = `#!/bin/sh
if [ "$1" = "--appimage-extract" ]; then
  [ "$FAKE_EXTRACT_FAIL" = "1" ] && exit 1
  mkdir -p "squashfs-root/$(dirname "$2")" && printf PNG > "squashfs-root/$2"
  exit 0
fi
exit 0
`

type imageEnv struct {
	t               *testing.T
	root, home, bin string
	data            string
	download        string
	store           *fakeAppStore
	downloads       int
	deb             debPlacer
	ai              appInstaller
	plat            linuxPlatform
}

func newImageEnv(t *testing.T) *imageEnv {
	t.Helper()
	root := t.TempDir()
	e := &imageEnv{t: t, root: root, home: filepath.Join(root, "home"), store: &fakeAppStore{}}
	e.bin, e.data = filepath.Join(e.home, ".local", "bin"), filepath.Join(e.home, ".local", "share")
	festivalHome := filepath.Join(e.home, "installer")
	t.Setenv("FESTIVAL_HOME", festivalHome)
	if err := os.MkdirAll(festivalHome, 0o700); err != nil {
		t.Fatal(err)
	}
	e.download = filepath.Join(root, "Festival.App_0.2.0_amd64.AppImage")
	if err := os.WriteFile(e.download, []byte(fakeAppImageScript), 0o644); err != nil {
		t.Fatal(err)
	}
	e.deb = debPlacer{
		run: func(context.Context, time.Duration, string, io.Writer, string, ...string) (appCommandResult, error) {
			t.Error("the deb runner must not be called")
			return appCommandResult{}, errors.New("unexpected")
		},
		probe:  func(context.Context) (debInstall, error) { return debInstall{}, nil },
		exists: func(string) bool { return true },
	}
	e.plat = linuxPlatform{
		deb:      e.deb,
		appimage: appImagePlacer{run: runAppCommand, binDir: e.bin, dataHome: e.data, stageRoot: festivalHome},
		hasApt:   true,
	}
	e.ai = appInstaller{
		goos: "linux", euid: func() int { return 1000 }, platform: e.plat,
		resolve: func(context.Context, string, string, source.VerifyOptions) (appRelease, error) {
			rel := appRelease{PackageID: AppPackageID, Version: "0.2.0", Channel: "stable", Source: "official-obey", Format: "appimage", BundleID: AppBundleID}
			sum := sha256.Sum256([]byte(fakeAppImageScript))
			rel.Artifact.URL, rel.Artifact.Sha256, rel.Artifact.Filename = "https://example.invalid/x.AppImage", hex.EncodeToString(sum[:]), "Festival.App_0.2.0_amd64.AppImage"
			return rel, nil
		},
		download: func(_ context.Context, _ string, rel appRelease, _ appDownloadOptions) (verifiedAppArtifact, error) {
			e.downloads++
			return verifiedAppArtifact{Path: e.download, Dir: "", Release: rel}, nil
		},
		begin: beginAppTxn, refuseRunning: refuseRunningApp, receipts: e.store.factory(),
	}
	return e
}

func (e *imageEnv) install() (AppResult, error) {
	return e.ai.install(context.Background(), AppInstallOptions{Format: "appimage"})
}
func (e *imageEnv) target() string { return filepath.Join(e.bin, "festival-app") }
func (e *imageEnv) desktop() string {
	return filepath.Join(e.data, "applications", "festival-app.desktop")
}
func (e *imageEnv) icon() string {
	return filepath.Join(e.data, "icons", "hicolor", "512x512", "apps", "festival-app.png")
}

func (e *imageEnv) writeTarget(content string) string {
	e.t.Helper()
	if err := os.MkdirAll(e.bin, 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(e.target(), []byte(content), 0o755); err != nil {
		e.t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func (e *imageEnv) noTemps() {
	e.t.Helper()
	for _, dir := range []string{e.bin, filepath.Dir(e.desktop()), filepath.Dir(e.icon())} {
		entries, _ := os.ReadDir(dir)
		for _, en := range entries {
			if strings.Contains(en.Name(), ".festival-") {
				e.t.Fatalf("temp left in %s: %s", dir, en.Name())
			}
		}
	}
}

func TestAppImageFreshInstallPlacesBinaryDesktopEntryAndIcon(t *testing.T) {
	e := newImageEnv(t)
	res, err := e.install()
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(e.target())
	if err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("binary: %v %v", fi, err)
	}
	want, _ := desktopEntry(e.target())
	if got, _ := os.ReadFile(e.desktop()); string(got) != want {
		t.Fatalf("desktop entry:\n%s", got)
	}
	if got, _ := os.ReadFile(e.icon()); string(got) != "PNG" {
		t.Fatalf("icon = %q", got)
	}
	if e.store.r == nil || e.store.r.Location != e.target() || e.store.r.Executable != e.target() || e.store.r.Format != "appimage" || res.Location != e.target() || res.Adopted {
		t.Fatalf("res=%+v receipt=%+v", res, e.store.r)
	}
	e.noTemps()
}

func TestAppImageIconExtractionFailureIsANote(t *testing.T) {
	e := newImageEnv(t)
	t.Setenv("FAKE_EXTRACT_FAIL", "1")
	res, err := e.install()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(res.Notes, "\n"), "Could not extract the Festival App icon; the menu entry uses a generic icon.") {
		t.Fatalf("notes = %v", res.Notes)
	}
	if _, statErr := os.Stat(e.icon()); !os.IsNotExist(statErr) {
		t.Fatal("an icon was written")
	}
	if _, statErr := os.Stat(e.target()); statErr != nil {
		t.Fatal("binary missing")
	}
	e.noTemps()
}

func TestAppImageReplacesReceiptOwnedCopy(t *testing.T) {
	e := newImageEnv(t)
	sha := e.writeTarget("old appimage")
	e.store.r = &appReceipt{Version: "0.1.0", Format: "appimage", Location: e.target(), ArtifactSHA256: sha}
	res, err := e.install()
	if err != nil || res.Adopted || !res.Changed || res.PreviousVersion != "0.1.0" {
		t.Fatalf("%+v %v", res, err)
	}
	if got, _ := os.ReadFile(e.target()); string(got) != fakeAppImageScript {
		t.Fatal("binary not replaced")
	}
	e.noTemps()
}

func TestAppImageRefusesUnknownFileAtTarget(t *testing.T) {
	e := newImageEnv(t)
	e.writeTarget("someone else's festival-app")
	_, err := e.install()
	if errpkg.Code(err) != "E_APP_FOREIGN" || e.downloads != 0 {
		t.Fatalf("err=%v downloads=%d", err, e.downloads)
	}
	if got, _ := os.ReadFile(e.target()); string(got) != "someone else's festival-app" {
		t.Fatal("foreign file was changed")
	}
}

func TestAppImageSelfUpdatedCopyIsLeftAlone(t *testing.T) {
	e := newImageEnv(t)
	e.writeTarget("updated by the app itself")
	e.store.r = &appReceipt{Version: "0.1.0", Format: "appimage", Location: e.target(), ArtifactSHA256: strings.Repeat("c", 64)}
	res, err := e.install()
	if err != nil || res.Changed || e.downloads != 0 || len(res.Notes) != 1 {
		t.Fatalf("%+v %v downloads=%d", res, err, e.downloads)
	}
	if got, _ := os.ReadFile(e.target()); string(got) != "updated by the app itself" {
		t.Fatal("self-updated copy was changed")
	}
}

func TestAppImageKeepsForeignDesktopEntry(t *testing.T) {
	e := newImageEnv(t)
	if err := os.MkdirAll(filepath.Dir(e.desktop()), 0o755); err != nil {
		t.Fatal(err)
	}
	const mine = "[Desktop Entry]\nName=My own entry\n"
	if err := os.WriteFile(e.desktop(), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := e.install()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(e.desktop()); string(got) != mine {
		t.Fatal("foreign desktop entry was overwritten")
	}
	if !strings.Contains(strings.Join(res.Notes, "\n"), "Left your existing "+e.desktop()+" unchanged.") {
		t.Fatalf("notes = %v", res.Notes)
	}
}

func TestAppImageReceiptFailureRemovesFreshExtras(t *testing.T) {
	e := newImageEnv(t)
	boom := errors.New("receipt store down")
	e.store.failWrite = boom
	_, err := e.install()
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	for _, p := range []string{e.target(), e.desktop(), e.icon()} {
		if _, statErr := os.Lstat(p); !os.IsNotExist(statErr) {
			t.Fatalf("%s left behind: %v", p, statErr)
		}
	}
	e.noTemps()
}

func TestAppImageRunningAppRefused(t *testing.T) {
	t.Run("by executable", func(t *testing.T) {
		e := newImageEnv(t)
		e.writeTarget("placeholder")
		cmd := startSleeper(t, e.target())
		sum, err := artifacts.SHA256(context.Background(), e.target())
		if err != nil {
			t.Fatal(err)
		}
		e.store.r = &appReceipt{Version: "0.1.0", Format: "appimage", Location: e.target(), ArtifactSHA256: sum}
		_ = waitRunning(t, appRunScope{Exes: []string{e.target()}}, cmd.Process.Pid)
		_, err = e.install()
		if errpkg.Code(err) != "E_APP_RUNNING" || e.downloads != 0 {
			t.Fatalf("err=%v downloads=%d", err, e.downloads)
		}
	})
	t.Run("by APPIMAGE environment", func(t *testing.T) {
		e := newImageEnv(t)
		sum := e.writeTarget("placeholder")
		e.store.r = &appReceipt{Version: "0.1.0", Format: "appimage", Location: e.target(), ArtifactSHA256: sum}
		cmd := startSleeper(t, filepath.Join(e.root, "mount", "usr", "bin", "runtime"), "APPIMAGE="+e.target())
		_ = waitRunning(t, appRunScope{Exes: []string{e.target()}}, cmd.Process.Pid)
		_, err := e.install()
		if errpkg.Code(err) != "E_APP_RUNNING" || e.downloads != 0 {
			t.Fatalf("err=%v downloads=%d", err, e.downloads)
		}
	})
}

func TestAppLinuxPlatformFormatAppImageOnAptSystem(t *testing.T) {
	e := newImageEnv(t)
	f, err := e.plat.format(context.Background(), "appimage", nil)
	if err != nil || f != "appimage" {
		t.Fatalf("%q %v", f, err)
	}
	if _, err := e.install(); err != nil {
		t.Fatal(err)
	}
}
