//go:build container_fs && linux

package app

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/apptest/appfixture"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

type updEnv struct {
	t          *testing.T
	f          *appFixture
	rel        *appfixture.Releases
	releases   []metadata.Release
	registered bool
}

func newUpdEnv(t *testing.T) *updEnv {
	t.Helper()
	e := &updEnv{t: t, f: newAppFixture(t), rel: appfixture.NewReleases(t)}
	t.Cleanup(WithUpdaterKeyForTest(e.rel.Key))
	t.Cleanup(source.WithPinnedKeyStoreForTest(e.f.Verify.KeyStore))
	return e
}

func (e *updEnv) vo() source.VerifyOptions { return source.DefaultVerifyOptions(io.Discard, false) }

// publish adds a stable release whose AppImage holds body, and commits the catalog.
func (e *updEnv) publish(version string, body []byte) {
	e.t.Helper()
	e.releases = append(e.releases, appFixtureRelease(version, "stable", e.rel.Artifact("appimage", version, body)))
	e.f.publish(appFixtureManifest(e.releases...), appFixtureOptions{})
	if !e.registered {
		e.f.register("official-obey")
		e.registered = true
	}
}

func (e *updEnv) install() AppResult {
	e.t.Helper()
	res, err := InstallApp(context.Background(), AppInstallOptions{Format: "appimage", Verify: e.vo()})
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func (e *updEnv) row() PackageUpdate {
	e.t.Helper()
	res, err := CheckUpdates(context.Background(), e.vo())
	if err != nil {
		e.t.Fatal(err)
	}
	for _, p := range res.Packages {
		if p.Package == AppPackageID {
			return p
		}
	}
	e.t.Fatalf("no App row in %+v", res.Packages)
	return PackageUpdate{}
}

func TestAppCheckUpdateReportsAvailableWithOnDiskVersion(t *testing.T) {
	e := newUpdEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	res := e.install()
	e.publish("0.2.1", []byte("appimage 0.2.1"))
	row := e.row()
	if row.Status != UpdateAvailable || row.Installed != "0.2.0" || row.Latest != "0.2.1" {
		t.Fatalf("%+v", row)
	}
	t.Logf("outdated row: %+v (installed at %s)", row, res.Location)
}

func TestAppCheckUpdateSelfUpdatedAppImageIsCurrent(t *testing.T) {
	e := newUpdEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	res := e.install()
	if err := os.WriteFile(res.Location, []byte("the App replaced itself"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.publish("0.2.1", []byte("appimage 0.2.1"))
	row := e.row()
	if row.Status != UpdateCurrent || !strings.Contains(row.Detail, "Festival App updated itself; festival leaves it in place") {
		t.Fatalf("%+v", row)
	}
}

func TestAppCheckUpdateNeverOffersDowngrade(t *testing.T) {
	e := newUpdEnv(t)
	rel := appFixtureRelease("0.2.1", "stable", e.rel.Artifact("deb", "0.2.1", []byte("deb 0.2.1")))
	e.releases = append(e.releases, rel)
	e.f.publish(appFixtureManifest(e.releases...), appFixtureOptions{})
	e.f.register("official-obey")
	statusEnvKeep(t)
	if err := storeFor(t).Write(context.Background(), appReceipt{
		Version: "0.2.0", Source: "official-obey", Channel: "stable", Format: "deb", Location: "dpkg:festival-app",
		BundleID: AppBundleID, ArtifactSHA256: strings.Repeat("a", 64), Executable: "/usr/bin/festival-app", Mode: 0o755,
	}); err != nil {
		t.Fatal(err)
	}
	prev := appPlatformFor
	appPlatformFor = func(context.Context) (appPlatform, error) {
		return linuxPlatform{deb: debPlacer{probe: func(context.Context) (debInstall, error) { return debInstall{Installed: true, Version: "0.3.0"}, nil }}}, nil
	}
	t.Cleanup(func() { appPlatformFor = prev })
	row := e.row()
	if row.Status != UpdateCurrent || row.Installed != "0.3.0" || row.Latest != "0.2.1" {
		t.Fatalf("%+v", row)
	}
}

// statusEnvKeep makes the fixture's FESTIVAL_HOME exist so storeFor can open it.
func statusEnvKeep(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(os.Getenv("FESTIVAL_HOME"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestAppCheckUpdateMissingCopyIsError(t *testing.T) {
	e := newUpdEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	res := e.install()
	if err := os.Remove(res.Location); err != nil {
		t.Fatal(err)
	}
	row := e.row()
	if row.Status != UpdateCheckFailed || !strings.Contains(row.Detail, "E_UPDATE_RECEIPT") || !strings.Contains(row.Detail, "Festival App is missing from "+res.Location+"; run festival install app") {
		t.Fatalf("%+v", row)
	}
}

func TestAppUpdateInstalledDispatchesBeforeOwnershipCheck(t *testing.T) {
	e := newUpdEnv(t)
	e.publish("0.2.0", []byte("appimage 0.2.0"))
	e.install()
	e.publish("0.2.1", []byte("appimage 0.2.1"))
	res, _, err := UpdateInstalled(context.Background(), AppPackageID, UpdateOptions{Verify: e.vo()})
	if err != nil {
		t.Fatalf("an App receipt (hash empty) must not fail the plugin ownership check: %v", err)
	}
	if res.Action != "upgraded" || res.Version != "0.2.1" || res.From != "0.2.0" {
		t.Fatalf("%+v", res)
	}
}
