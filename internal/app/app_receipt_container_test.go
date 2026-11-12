//go:build container_fs && linux

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

func hexSHA(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// statusEnv isolates HOME and FESTIVAL_HOME under a temp dir.
func statusEnv(t *testing.T) (home, festivalHome string) {
	t.Helper()
	home = t.TempDir()
	festivalHome = filepath.Join(home, "installer")
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", festivalHome)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	return home, festivalHome
}

func storeFor(t *testing.T) dbAppReceiptStore {
	t.Helper()
	ctx := context.Background()
	if err := state.EnsureHome(ctx, 0o700); err != nil {
		t.Fatal(err)
	}
	home, _ := state.Home(ctx)
	return dbAppReceiptStore{db: openTestDB(t, home)}
}

func imageReceipt(path string, content []byte) appReceipt {
	return appReceipt{
		Version: "0.2.0", Source: "official-obey", Channel: "stable", ManifestURL: "https://example.invalid/x",
		Format: "appimage", Location: path, BundleID: AppBundleID, ArtifactSHA256: hexSHA(content), Executable: path, Mode: 0o755,
	}
}

func writeImage(t *testing.T, home string, content []byte) string {
	t.Helper()
	path := filepath.Join(home, ".local", "bin", "festival-app")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAppReceiptStoreWritesExactD006Shape(t *testing.T) {
	statusEnv(t)
	s := storeFor(t)
	r := imageReceipt("/h/.local/bin/festival-app", []byte("x"))
	r.Adopted = true
	if err := s.Write(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	var version, src, channel string
	if err := s.db.QueryRow("SELECT version, source, channel FROM receipts WHERE package_id = ?", AppPackageID).Scan(&version, &src, &channel); err != nil {
		t.Fatal(err)
	}
	if version != "0.2.0" || src != "official-obey" || channel != "stable" {
		t.Fatalf("receipts row: %s %s %s", version, src, channel)
	}
	var path, hash string
	var mode int64
	rows := 0
	files, err := s.db.Query("SELECT path, hash, mode FROM receipt_files WHERE receipt_pkg_id = ?", AppPackageID)
	if err != nil {
		t.Fatal(err)
	}
	for files.Next() {
		rows++
		if err := files.Scan(&path, &hash, &mode); err != nil {
			t.Fatal(err)
		}
	}
	_ = files.Close()
	if rows != 1 || path != "/h/.local/bin/festival-app" || hash != "" || mode != 0o755 {
		t.Fatalf("receipt_files: %d rows, %q %q %o", rows, path, hash, mode)
	}
	meta := map[string]string{}
	mrows, err := s.db.Query("SELECT key, value FROM receipt_metadata WHERE receipt_pkg_id = ?", AppPackageID)
	if err != nil {
		t.Fatal(err)
	}
	for mrows.Next() {
		var k, v string
		if err := mrows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		meta[k] = v
	}
	_ = mrows.Close()
	want := map[string]string{
		"app.format": "appimage", "app.location": "/h/.local/bin/festival-app", "app.bundle_id": AppBundleID,
		"app.artifact_sha256": r.ArtifactSHA256, "app.adopted": "true", "manifest_url": "https://example.invalid/x",
	}
	if len(meta) != len(want) {
		t.Fatalf("metadata = %v", meta)
	}
	for k, v := range want {
		if meta[k] != v {
			t.Fatalf("metadata[%s] = %q, want %q", k, meta[k], v)
		}
	}
	back, err := s.Read(context.Background())
	if err != nil || back == nil || *back != r {
		t.Fatalf("read back %+v %v", back, err)
	}
}

func TestAppReceiptStoreRefusesManagedRoots(t *testing.T) {
	statusEnv(t)
	s := storeFor(t)
	s.binDir = filepath.Join(t.TempDir(), "installer", "bin")
	s.pluginsRoot = filepath.Join(t.TempDir(), ".obey", "plugins")
	for _, exe := range []string{filepath.Join(s.binDir, "festival-app"), filepath.Join(s.pluginsRoot, "festival-app", "bin", "festival-app")} {
		r := imageReceipt(exe, []byte("x"))
		if err := s.Write(context.Background(), r); errpkg.Code(err) != "E_RECEIPT_PATH" {
			t.Errorf("%s: %v", exe, err)
		}
	}
	ok := imageReceipt(filepath.Join(t.TempDir(), "festival-app"), []byte("x"))
	if err := s.Write(context.Background(), ok); err != nil {
		t.Errorf("a path outside both roots must be accepted: %v", err)
	}
}

func TestAppInstallAppImageWritesReceiptReadByStatus(t *testing.T) {
	home, _ := statusEnv(t)
	ctx := context.Background()
	if err := state.EnsureHome(ctx, 0o700); err != nil {
		t.Fatal(err)
	}
	download := filepath.Join(t.TempDir(), "Festival.App.AppImage")
	if err := os.WriteFile(download, []byte(fakeAppImageScript), 0o644); err != nil {
		t.Fatal(err)
	}
	platform, err := hostAppPlatform(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := productionAppReceipts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ai := appInstaller{
		goos: "linux", euid: func() int { return 1000 }, platform: platform,
		resolve: func(context.Context, string, string, source.VerifyOptions) (appRelease, error) {
			rel := appRelease{PackageID: AppPackageID, Version: "0.2.0", Channel: "stable", Source: "official-obey", Format: "appimage", BundleID: AppBundleID}
			rel.Artifact.URL, rel.Artifact.Sha256, rel.Artifact.Filename = "https://example.invalid/x.AppImage", hexSHA([]byte(fakeAppImageScript)), "Festival.App.AppImage"
			return rel, nil
		},
		download: func(_ context.Context, _ string, rel appRelease, _ appDownloadOptions) (verifiedAppArtifact, error) {
			return verifiedAppArtifact{Path: download, Release: rel}, nil
		},
		begin: beginAppTxn, refuseRunning: refuseRunningApp, receipts: receipts,
	}
	if _, err := ai.install(ctx, AppInstallOptions{Format: "appimage"}); err != nil {
		t.Fatal(err)
	}
	rep, err := StatusReportFor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a := rep.App
	if !a.Installed || a.Format == nil || *a.Format != "appimage" || a.Version == nil || *a.Version != "0.2.0" ||
		a.Location == nil || *a.Location != filepath.Join(home, ".local", "bin", "festival-app") {
		t.Fatalf("status.app = %+v", a)
	}
	t.Logf("installed app object: %s", mustJSON(t, a))
}

func TestAppStatusSelfUpdatedAppImage(t *testing.T) {
	home, _ := statusEnv(t)
	orig := []byte("original")
	path := writeImage(t, home, orig)
	if err := storeFor(t).Write(context.Background(), imageReceipt(path, orig)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("updated by the app itself"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := resolveStatusApp(context.Background())
	if !a.Installed || a.Version != nil || a.UpdatedByApp == nil || !*a.UpdatedByApp {
		t.Fatalf("%+v", a)
	}
	t.Logf("self-updated app object: %s", mustJSON(t, a))
}

func TestAppStatusRemovedOutsideFestival(t *testing.T) {
	home, _ := statusEnv(t)
	path := writeImage(t, home, []byte("x"))
	if err := storeFor(t).Write(context.Background(), imageReceipt(path, []byte("x"))); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	a := resolveStatusApp(context.Background())
	if a.Installed || a.ReceiptVersion == nil || *a.ReceiptVersion != "0.2.0" || a.Note == nil || *a.Note != "removed outside festival" || a.Version != nil {
		t.Fatalf("%+v", a)
	}
	t.Logf("removed app object: %s", mustJSON(t, a))
}

func TestAppStatusLatestFromCachedCatalogWithoutNetwork(t *testing.T) {
	setup := func(t *testing.T) *appFixture {
		f := newAppFixture(t)
		rel := appFixtureRelease("0.2.1", "stable",
			appFixtureArtifact("appimage", runtimeGOOS, runtimeGOARCH),
			appFixtureArtifact("deb", runtimeGOOS, runtimeGOARCH))
		f.publish(appFixtureManifest(rel), appFixtureOptions{})
		f.register("official-obey")
		content := []byte("installed 0.2.0")
		path := writeImage(t, f.Home, content)
		if err := storeFor(t).Write(context.Background(), imageReceipt(path, content)); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(f.Repo, f.Repo+".moved"); err != nil {
			t.Fatal(err)
		}
		return f
	}
	t.Run("trusted", func(t *testing.T) {
		f := setup(t)
		t.Cleanup(source.WithPinnedKeyStoreForTest(f.Verify.KeyStore))
		a := resolveStatusApp(context.Background())
		if !a.Installed || a.Latest == nil || *a.Latest != "0.2.1" || a.UpdateAvailable == nil || !*a.UpdateAvailable {
			t.Fatalf("%+v", a)
		}
		t.Logf("update-available app object: %s", mustJSON(t, a))
	})
	t.Run("untrusted", func(t *testing.T) {
		setup(t)
		a := resolveStatusApp(context.Background())
		if !a.Installed || a.Latest != nil || a.UpdateAvailable != nil {
			t.Fatalf("an untrusted catalog must not break status: %+v", a)
		}
	})
}

func TestAppStatusAbsentCreatesNothing(t *testing.T) {
	_, festivalHome := statusEnv(t)
	a := resolveStatusApp(context.Background())
	if a.Installed || a.Version != nil || a.Note != nil {
		t.Fatalf("%+v", a)
	}
	if _, err := os.Stat(festivalHome); !os.IsNotExist(err) {
		t.Fatalf("status created the home: %v", err)
	}
}

func TestAppStatusLargeAppImageTiming(t *testing.T) {
	home, _ := statusEnv(t)
	path := writeImage(t, home, nil)
	if err := os.Truncate(path, 105<<20); err != nil {
		t.Fatal(err)
	}
	if err := storeFor(t).Write(context.Background(), imageReceipt(path, []byte("different"))); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	a := resolveStatusApp(context.Background())
	took := time.Since(start)
	t.Logf("resolveStatusApp on a 105 MiB AppImage took %s", took)
	if !a.Installed {
		t.Fatalf("%+v", a)
	}
	if took > probeTimeout {
		t.Fatalf("took %s, over the %s probe budget", took, probeTimeout)
	}
}
