//go:build windows && container_fs

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/artifacts"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func TestWindowsDoctorVerifiesThreeManagedExecutablesAcrossVersions(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	for _, version := range []string{"1.0.0", "2.0.0"} {
		t.Logf("stage: create and hash receipt-owned fixture files at version %s", version)
		var owned []receipts.OwnedFile
		for _, name := range []string{"camp.exe", "fest.exe", "festival.exe"} {
			path := filepath.Join(binDir, name)
			if err := os.WriteFile(path, []byte(version+"-"+name), 0o755); err != nil {
				t.Fatal(err)
			}
			hash, err := artifacts.SHA256(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			owned = append(owned, receipts.OwnedFile{Path: path, Hash: hash, Mode: 0o755})
		}
		t.Logf("stage: write receipt database at version %s", version)
		db, err := state.OpenDB(ctx, home)
		if err != nil {
			t.Fatal(err)
		}
		if err := receipts.Write(ctx, db.Raw(), receipts.Receipt{
			PackageID: FestivalPackageID, Version: version, Channel: "stable", Source: "official-obey", OwnedFiles: owned,
		}); err != nil {
			_ = db.Close(ctx)
			t.Fatal(err)
		}
		if err := db.Close(ctx); err != nil {
			t.Fatal(err)
		}
		t.Logf("stage: verify receipt integrity at version %s", version)
		check := checkReceiptsIntegrity(ctx)
		if check.Status != DoctorOK {
			t.Fatalf("doctor receipt check at %s = %+v", version, check)
		}
		t.Logf("stage: full Doctor with on-PATH invalid executable fixtures at version %s", version)
		if checks := Doctor(ctx); DoctorFailed(checks) {
			t.Fatalf("doctor failed for complete Windows suite at %s: %+v", version, checks)
		}
	}
	t.Log("stage: corrupt fest and verify receipt failure")
	if err := os.WriteFile(filepath.Join(binDir, "fest.exe"), []byte("corrupted"), 0o755); err != nil {
		t.Fatal(err)
	}
	if check := checkReceiptsIntegrity(ctx); check.Status != DoctorFail {
		t.Fatalf("doctor must fail after executable corruption: %+v", check)
	}
	t.Log("stage: full Doctor after corruption")
	if checks := Doctor(ctx); !DoctorFailed(checks) {
		t.Fatalf("doctor succeeded after executable corruption: %+v", checks)
	}
}
