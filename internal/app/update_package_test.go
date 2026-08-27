package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stubLatestSuite(t *testing.T, version string, err error) {
	t.Helper()
	orig := LookupLatestSuite
	LookupLatestSuite = func(context.Context, string) (string, error) {
		return version, err
	}
	t.Cleanup(func() { LookupLatestSuite = orig })
}

func writeVersionedPackagePrefix(t *testing.T, root, version string) string {
	t.Helper()
	bin, _ := writePackagePrefix(t, root)
	script := "#!/bin/sh\necho " + version + "\n"
	for _, name := range []string{"camp", "fest", "festival"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

func TestUpdateFestival_PackageOriginReportsAvailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	bin := writeVersionedPackagePrefix(t, t.TempDir(), "0.3.1")
	t.Setenv("PATH", bin)
	stubLatestSuite(t, "0.3.3", nil)

	res, warning, err := UpdateFestival(context.Background(), UpdateOptions{})
	if err != nil {
		t.Fatalf("package update must not error, got %v", err)
	}
	if res.Action != "package" {
		t.Fatalf("Action=%q, want package", res.Action)
	}
	if res.Version != "0.3.1" {
		t.Fatalf("Version=%q, want 0.3.1", res.Version)
	}
	if res.Latest != "0.3.3" {
		t.Fatalf("Latest=%q, want 0.3.3", res.Latest)
	}
	if !strings.Contains(warning, "update available: 0.3.1 -> 0.3.3") {
		t.Fatalf("warning missing update available, got %q", warning)
	}
	if !strings.Contains(warning, "upgrade with:") {
		t.Fatalf("warning missing upgrade command, got %q", warning)
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatal("package update must not create state.db")
	}
}

func TestUpdateFestival_PackageOriginCurrent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	bin := writeVersionedPackagePrefix(t, t.TempDir(), "0.3.3")
	t.Setenv("PATH", bin)
	stubLatestSuite(t, "0.3.3", nil)

	res, warning, err := UpdateFestival(context.Background(), UpdateOptions{})
	if err != nil {
		t.Fatalf("package update must not error, got %v", err)
	}
	if res.Action != "package" || res.Latest != "0.3.3" {
		t.Fatalf("got action=%q latest=%q", res.Action, res.Latest)
	}
	if strings.Contains(warning, "update available") {
		t.Fatalf("current install must not claim update available: %q", warning)
	}
	if !strings.Contains(warning, "already current at 0.3.3") {
		t.Fatalf("warning=%q", warning)
	}
}

func TestUpdateFestival_PackageOriginLookupFailStillPackage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	bin := writeVersionedPackagePrefix(t, t.TempDir(), "0.3.1")
	t.Setenv("PATH", bin)
	stubLatestSuite(t, "", errors.New("offline"))

	res, warning, err := UpdateFestival(context.Background(), UpdateOptions{})
	if err != nil {
		t.Fatalf("lookup fail must not fail update, got %v", err)
	}
	if res.Action != "package" {
		t.Fatalf("Action=%q, want package", res.Action)
	}
	if res.Latest != "" {
		t.Fatalf("Latest=%q, want empty", res.Latest)
	}
	if !strings.Contains(warning, "upgrade with:") {
		t.Fatalf("warning=%q", warning)
	}
	if strings.Contains(warning, "update available") {
		t.Fatalf("no latest must not claim update available: %q", warning)
	}
}

func TestUpdateFestival_PackageOriginForceStillDoesNotPlant(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	bin := writeVersionedPackagePrefix(t, t.TempDir(), "0.3.1")
	t.Setenv("PATH", bin)
	stubLatestSuite(t, "0.3.3", nil)

	res, _, err := UpdateFestival(context.Background(), UpdateOptions{Force: true})
	if err != nil {
		t.Fatalf("force package update must not error, got %v", err)
	}
	if res.Action != "package" {
		t.Fatalf("Action=%q, want package (update never force-writes)", res.Action)
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatal("force package update must not create state.db")
	}
}

func TestUpdateFestival_DualReportsPackage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(home, "bin", "camp"))
	bin := writeVersionedPackagePrefix(t, t.TempDir(), "0.3.1")
	t.Setenv("PATH", bin)
	stubLatestSuite(t, "0.3.3", nil)

	res, warning, err := UpdateFestival(context.Background(), UpdateOptions{})
	if err != nil {
		t.Fatalf("Dual update must not error, got %v", err)
	}
	if res.Action != "package" {
		t.Fatalf("Action=%q, want package", res.Action)
	}
	if !strings.Contains(warning, "update available: 0.3.1 -> 0.3.3") {
		t.Fatalf("warning=%q", warning)
	}
}

func TestPackageUpdateWarning(t *testing.T) {
	got := packageUpdateWarning("0.3.1", "0.3.3", "yay -Syu festival-bin")
	if !strings.Contains(got, "update available: 0.3.1 -> 0.3.3") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "upgrade with: yay -Syu festival-bin") {
		t.Fatalf("got %q", got)
	}
	got = packageUpdateWarning("0.3.3", "0.3.3", "yay -Syu festival-bin")
	if strings.Contains(got, "update available") {
		t.Fatalf("got %q", got)
	}
}
