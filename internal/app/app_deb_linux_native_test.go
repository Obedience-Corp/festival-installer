//go:build linux && app_native

package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/apptest"
)

const debReleaseBase = "https://github.com/Obedience-Corp/obey-releases/releases/download/app-test-25628d122be1ab6795737af192171d227ec3f2f2/"

func debGet(t *testing.T, name, dest string) {
	t.Helper()
	resp, err := http.Get(debReleaseBase + name)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", name, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, resp.Body); err != nil {
		t.Fatal(err)
	}
}

// TestAppDebNativeRealPackageInstalls places the real 0.1.7 deb with the real
// pkexec and apt-get on a fresh hosted ubuntu runner whose polkit rule
// authorizes the runner user.
func nativeInstallDeb(t *testing.T) debPlacer {
	apptest.RequireHostedRunner(t)
	const asset = "Festival.App_0.1.7_amd64.deb"
	dir := t.TempDir()
	deb, sums := filepath.Join(dir, asset), filepath.Join(dir, "SHA256SUMS")
	debGet(t, asset, deb)
	debGet(t, "SHA256SUMS", sums)
	want := ""
	f, err := os.Open(sums)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			want = fields[0]
		}
	}
	_ = f.Close()
	raw, err := os.ReadFile(deb)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); want == "" || hex.EncodeToString(sum[:]) != want {
		t.Fatalf("deb sha256 does not match SHA256SUMS (want %q)", want)
	}

	t.Setenv("FESTIVAL_HOME", filepath.Join(dir, "home"))
	home := os.Getenv("FESTIVAL_HOME")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	db := openTestDB(t, home)
	tx, err := beginAppTxn(context.Background(), db, home)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Close(context.Background()) }()
	rel := appRelease{PackageID: AppPackageID, Version: "0.1.7", Channel: "stable", Source: "official-obey", Format: "deb", BundleID: AppBundleID}
	rel.Artifact.Sha256, rel.Artifact.Filename = want, asset
	d := debPlacer{
		run:        runAppCommand,
		probe:      func(ctx context.Context) (debInstall, error) { return probeDeb(ctx, runDpkgQuery) },
		exists:     executableExists,
		tempRoot:   os.TempDir(),
		executable: "/usr/bin/festival-app",
	}
	if _, err := d.place(context.Background(), tx, appPlaceRequest{
		Release: rel, Artifact: verifiedAppArtifact{Path: deb, Dir: dir, Release: rel}, Output: os.Stderr,
		WriteReceipt: func(context.Context, *sql.DB, appPlacement) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	inst, err := probeDeb(context.Background(), runDpkgQuery)
	if err != nil || !inst.Installed || inst.Version != "0.1.7" {
		t.Fatalf("dpkg reports %+v %v", inst, err)
	}
	if _, err := os.Stat("/usr/bin/festival-app"); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAppDebNativeRealPackageInstalls(t *testing.T) {
	apptest.RequireHostedRunner(t)
	nativeInstallDeb(t)
}

// TestAppDebNativeRemove removes the real package it just installed.
func TestAppDebNativeRemove(t *testing.T) {
	apptest.RequireHostedRunner(t)
	d := nativeInstallDeb(t)
	removed, _, err := d.remove(context.Background(), appReceipt{Version: "0.1.7", Format: "deb", Location: "dpkg:festival-app"}, os.Stderr)
	if err != nil || !removed {
		t.Fatalf("remove: %v %v", removed, err)
	}
	inst, err := probeDeb(context.Background(), runDpkgQuery)
	if err != nil || inst.Installed {
		t.Fatalf("dpkg still lists festival-app: %+v %v", inst, err)
	}
}
