package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatus_PackageFillsChannelCard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	bin, shell := writePackagePrefix(t, t.TempDir())
	t.Setenv("PATH", bin)

	sum, err := Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if sum.Action != "package" {
		t.Fatalf("Action=%q, want package", sum.Action)
	}
	if sum.Flavor != FlavorUnknown {
		t.Fatalf("Flavor=%q, want unknown", sum.Flavor)
	}
	if sum.Prefix != bin {
		t.Fatalf("Prefix=%q, want %q", sum.Prefix, bin)
	}
	if !strings.Contains(sum.Helper, filepath.Join(shell, "festival.zsh")) {
		t.Fatalf("Helper=%q, want packaged helper", sum.Helper)
	}
	if sum.Upgrade == "" {
		t.Fatal("Upgrade must be set for package origin")
	}
	if strings.Contains(sum.Upgrade, "yay") {
		t.Fatalf("unknown flavor must not print yay, got %q", sum.Upgrade)
	}
}

func TestStatus_LeftoverFillsShadowPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	bin := t.TempDir()
	writeStub(t, filepath.Join(bin, "camp"))
	t.Setenv("PATH", bin)

	sum, err := Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if sum.Action != "unmanaged" {
		t.Fatalf("Action=%q, want unmanaged", sum.Action)
	}
	if len(sum.Shadows) == 0 {
		t.Fatal("leftover Status must list leftover tool paths in Shadows")
	}
}
