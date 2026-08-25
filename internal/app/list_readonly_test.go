package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/source"
)

func TestList_EmptyExistingHomeDoesNotCreateDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("PATH", t.TempDir())

	got, err := ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(got.Packages) != 0 {
		t.Fatalf("want no packages, got %+v", got.Packages)
	}
	assertNoInstallerState(t, home)
}

func TestList_PackageSyntheticRow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	bin, _ := writePackagePrefix(t, t.TempDir())
	t.Setenv("PATH", bin)

	got, err := ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled: %v", err)
	}
	if len(got.Packages) != 1 {
		t.Fatalf("want synthetic suite row, got %+v", got.Packages)
	}
	row := got.Packages[0]
	if row.Origin != "package" {
		t.Fatalf("Origin=%q, want package", row.Origin)
	}
	if row.PackageID != FestivalPackageID {
		t.Fatalf("PackageID=%q", row.PackageID)
	}
	if row.Source == "" {
		t.Fatal("synthetic row needs Source")
	}
	assertNoInstallerState(t, home)
}

func TestListMarketplacesIfExists_EmptyHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	views, err := source.ListMarketplacesIfExists(context.Background(), source.DefaultVerifyOptions(nil, false))
	if err != nil {
		t.Fatalf("ListMarketplacesIfExists: %v", err)
	}
	if len(views) != 0 {
		t.Fatalf("want no views, got %+v", views)
	}
	assertNoInstallerState(t, home)
}

func TestDoctor_EmptyExistingHomeDoesNotCreateDB(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("PATH", t.TempDir())
	_ = Doctor(context.Background())
	assertNoInstallerState(t, home)
}

func assertNoInstallerState(t *testing.T, home string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatalf("state.db must not exist, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "locks")); !os.IsNotExist(err) {
		t.Fatalf("locks/ must not exist, err=%v", err)
	}
}
