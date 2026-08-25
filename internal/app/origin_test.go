package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectSuite_EmptyHomeDoesNotCreateState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("PATH", t.TempDir()) // no suite tools

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if got.Kind != OriginAbsent {
		t.Fatalf("Kind=%q, want absent", got.Kind)
	}
	if got.Dual {
		t.Fatal("Dual should be false with no BinDir files")
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatalf("state.db must not exist, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "locks")); !os.IsNotExist(err) {
		t.Fatalf("locks/ must not exist, stat err=%v", err)
	}
}

func TestDetectSuite_LeftoverOnPATH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)

	bin := t.TempDir()
	writeStub(t, filepath.Join(bin, "camp"))
	t.Setenv("PATH", bin)

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if got.Kind != OriginLeftover {
		t.Fatalf("Kind=%q, want leftover", got.Kind)
	}
	if got.Prefix != bin {
		t.Fatalf("Prefix=%q, want %q", got.Prefix, bin)
	}
	if got.Dual {
		t.Fatal("Dual should be false: no BinDir files")
	}
	if _, err := os.Stat(filepath.Join(home, "state.db")); !os.IsNotExist(err) {
		t.Fatalf("state.db must not exist, stat err=%v", err)
	}
}

func TestDetectSuite_ManagedAndDual(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	binDir := filepath.Join(home, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(binDir, "camp"))

	other := t.TempDir()
	writeStub(t, filepath.Join(other, "fest"))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+other)

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if got.Kind != OriginManaged {
		t.Fatalf("Kind=%q, want managed", got.Kind)
	}
	if !got.Dual {
		t.Fatal("want Dual: managed BinDir plus another PATH copy")
	}
}

func TestDetectSuite_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := DetectSuite(ctx)
	if err == nil {
		t.Fatal("want cancelled error")
	}
	if got.Kind != OriginAbsent {
		t.Fatalf("Kind=%q, want absent on cancel", got.Kind)
	}
}

func TestDetectSuite_PackageHelperAdjacentUnknown(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)

	bin, _ := writePackagePrefix(t, t.TempDir())
	t.Setenv("PATH", bin)

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if got.Kind != OriginPackage {
		t.Fatalf("Kind=%q, want package", got.Kind)
	}
	if got.Flavor != FlavorUnknown {
		t.Fatalf("Flavor=%q, want unknown (host pacman must not leak)", got.Flavor)
	}
	if strings.Contains(got.Upgrade, "yay") {
		t.Fatalf("unknown flavor must not print yay, got %q", got.Upgrade)
	}
}

func TestClassifyPath_LeftoverHomeLocalBin(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	bin := filepath.Join(user, "local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	camp := filepath.Join(bin, "camp")
	writeStub(t, camp)
	kind, flavor, _ := classifyPath(context.Background(), camp)
	if kind != OriginLeftover {
		t.Fatalf("Kind=%q flavor=%q, want leftover", kind, flavor)
	}
}

func TestClassifyPath_LinuxUsrLocalWithoutBrew(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin treats /usr/local as Homebrew when brew exists")
	}
	t.Setenv("HOMEBREW_PREFIX", "")
	kind, flavor, _ := classifyPath(context.Background(), "/usr/local/bin/camp")
	if kind == OriginPackage && flavor == FlavorHomebrew {
		t.Fatal("linux /usr/local/bin without brew must not be homebrew")
	}
}

func TestDetectSuite_PackagePlusBinDirIsDual(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	managed := filepath.Join(home, "bin")
	if err := os.MkdirAll(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(managed, "camp"))

	root := t.TempDir()
	bin, _ := writePackagePrefix(t, root)
	t.Setenv("PATH", bin)

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if got.Kind != OriginPackage {
		t.Fatalf("Kind=%q, want package", got.Kind)
	}
	if !got.Dual {
		t.Fatal("want Dual when BinDir has files and active Kind is package")
	}
}

func TestDetectSuite_HomebrewPrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	prefix := t.TempDir()
	t.Setenv("HOMEBREW_PREFIX", prefix)
	bin := filepath.Join(prefix, "bin")
	shell := filepath.Join(prefix, "share", "festival", "shell")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(shell, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(bin, "camp"))
	if err := os.WriteFile(filepath.Join(shell, "festival.zsh"), []byte("# brew helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if got.Kind != OriginPackage || got.Flavor != FlavorHomebrew {
		t.Fatalf("got Kind=%q Flavor=%q, want package/homebrew", got.Kind, got.Flavor)
	}
}

func TestDetectSuite_NpmLayout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	pkg := filepath.Join(t.TempDir(), "node_modules", "@obedience-corp", "festival")
	bin := filepath.Join(pkg, "bin")
	shell := filepath.Join(pkg, "share", "festival", "shell")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(shell, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(bin, "camp"))
	if err := os.WriteFile(filepath.Join(shell, "festival.zsh"), []byte("# npm helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if got.Kind != OriginPackage || got.Flavor != FlavorNpm {
		t.Fatalf("got Kind=%q Flavor=%q, want package/npm", got.Kind, got.Flavor)
	}
}

func TestDetectSuite_PathEntryIsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	fileEntry := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileEntry, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	writeStub(t, filepath.Join(bin, "camp"))
	t.Setenv("PATH", fileEntry+string(os.PathListSeparator)+bin)

	got, err := DetectSuite(context.Background())
	if err != nil {
		t.Fatalf("file PATH entry should be skipped or classified, err=%v", err)
	}
	if got.Kind != OriginLeftover {
		t.Fatalf("Kind=%q, want leftover from %s", got.Kind, bin)
	}
}

func TestOriginSourcesHaveNoOpenDB(t *testing.T) {
	for _, name := range []string{"origin.go", "origin_classify.go", "origin_probe.go"} {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte("OpenDB")) || bytes.Contains(b, []byte("EnsureHome")) || bytes.Contains(b, []byte("ReadFestivalReceipt")) {
			t.Fatalf("%s must not call OpenDB/EnsureHome/ReadFestivalReceipt", name)
		}
	}
}

func writePackagePrefix(t *testing.T, root string) (bin string, shell string) {
	t.Helper()
	bin = filepath.Join(root, "usr", "bin")
	shell = filepath.Join(root, "usr", "share", "festival", "shell")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(shell, 0o755); err != nil {
		t.Fatal(err)
	}
	writeStub(t, filepath.Join(bin, "camp"))
	writeStub(t, filepath.Join(bin, "fest"))
	writeStub(t, filepath.Join(bin, "festival"))
	if err := os.WriteFile(filepath.Join(shell, "festival.zsh"), []byte("# helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return bin, shell
}

func writeStub(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
