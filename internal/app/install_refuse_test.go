package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallFestival_RefusesPackageOrigin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	root := t.TempDir()
	bin, _ := writePackagePrefix(t, root)
	t.Setenv("PATH", bin)

	_, err := InstallFestival(context.Background(), InstallOptions{Channel: "stable"})
	if err == nil {
		t.Fatal("expected E_INSTALL_PACKAGE_CHANNEL")
	}
	if !strings.Contains(err.Error(), "E_INSTALL_PACKAGE_CHANNEL") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatal("refuse must not create state.db")
	}
}

func TestInstallFestival_LeftoverAllowed(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	bin := t.TempDir()
	writeStub(t, filepath.Join(bin, "camp"))
	t.Setenv("PATH", bin)
	err := refusePackageChannel(context.Background(), false)
	if err != nil {
		t.Fatalf("leftover-only must be allowed: %v", err)
	}
}
