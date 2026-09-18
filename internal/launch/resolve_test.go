package launch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestResolve_ManagedNotOnPATHPrefersManagedOverLeftover is the tie the leftover
// rule falls back on: neither stub answers a version probe, so neither copy can
// claim to be newer, and the copy the hub installed is the one to run.
func TestResolve_ManagedNotOnPATHPrefersManagedOverLeftover(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(bin, "camp")
	if err := os.WriteFile(managed, []byte("#!/bin/sh\necho managed\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	leftoverDir := t.TempDir()
	leftover := filepath.Join(leftoverDir, "camp")
	if err := os.WriteFile(leftover, []byte("#!/bin/sh\necho leftover\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Leftover is on PATH; managed BinDir is not.
	t.Setenv("PATH", leftoverDir)

	got, err := Resolve(context.Background(), "camp")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != managed {
		t.Fatalf("want managed %q, got %q (leftover was %q)", managed, got, leftover)
	}
}

// TestResolve_FreshManagedBeatsStaleLeftoverOnPATH is the scenario the managed
// preference exists for, told in versions rather than left to a tie: the hub
// just installed 0.10.1 into its own bin and the shell has not picked that
// directory up, so PATH still answers with the copy the install replaced.
func TestResolve_FreshManagedBeatsStaleLeftoverOnPATH(t *testing.T) {
	managed, leftoverDir := leftoverFixture(t, "v0.10.1-4-gd1fb37c7", "0.2.12")

	got, err := Resolve(context.Background(), "camp")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != managed {
		t.Fatalf("want the just-installed managed copy %q, got %q (leftover was %q)",
			managed, got, filepath.Join(leftoverDir, "camp"))
	}
}

// TestResolve_LiveLeftoverOnPATHBeatsStaleManaged is the same rule the other way
// round, and the one CI caught this file getting wrong: a dev machine whose
// ~/.obey/installer/bin has sat at 0.2.12 since an old install must not hide the
// camp the user actually builds and runs.
func TestResolve_LiveLeftoverOnPATHBeatsStaleManaged(t *testing.T) {
	_, leftoverDir := leftoverFixture(t, "0.2.12", "v0.10.1-4-gd1fb37c7")

	got, err := Resolve(context.Background(), "camp")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(leftoverDir, "camp")
	if got != want {
		t.Fatalf("want the live PATH copy %q, got %q", want, got)
	}
}

// leftoverFixture builds the leftover shape both directions share: a camp in the
// managed bin, a camp on PATH, the managed bin not on PATH, and each answering
// any version probe with the version it was given.
func leftoverFixture(t *testing.T, managedVersion, pathVersion string) (managed, leftoverDir string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	t.Setenv("HOMEBREW_PREFIX", "")
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	managed = filepath.Join(bin, "camp")
	writeVersionStub(t, managed, managedVersion)
	leftoverDir = t.TempDir()
	writeVersionStub(t, filepath.Join(leftoverDir, "camp"), pathVersion)
	t.Setenv("PATH", leftoverDir)
	return managed, leftoverDir
}

func writeVersionStub(t *testing.T, path, version string) {
	t.Helper()
	body := "#!/bin/sh\necho " + version + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil { //nolint:gosec // an executable fixture is the point
		t.Fatal(err)
	}
}

func TestResolve_PackageOriginPrefersPATH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	managedBin := filepath.Join(home, "bin")
	if err := os.MkdirAll(managedBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managedBin, "camp"), []byte("#!/bin/sh\necho managed\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	pkgBin := filepath.Join(root, "usr", "bin")
	pkgShell := filepath.Join(root, "usr", "share", "festival", "shell")
	if err := os.MkdirAll(pkgBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pkgShell, 0o755); err != nil {
		t.Fatal(err)
	}
	pkgCamp := filepath.Join(pkgBin, "camp")
	if err := os.WriteFile(pkgCamp, []byte("#!/bin/sh\necho package\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgBin, "fest"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgBin, "festival"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgShell, "festival.zsh"), []byte("# helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", pkgBin)

	got, err := Resolve(context.Background(), "camp")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != pkgCamp {
		t.Fatalf("want package PATH %q, got %q", pkgCamp, got)
	}
}

func TestResolve_FallsBackToPATH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("OBEY_INSTALLER_HOME", "")
	// empty managed bin
	_ = os.MkdirAll(filepath.Join(home, "bin"), 0o755)

	decoyDir := t.TempDir()
	decoy := filepath.Join(decoyDir, "fest")
	if err := os.WriteFile(decoy, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", decoyDir)

	got, err := Resolve(context.Background(), "fest")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != decoy {
		t.Fatalf("want PATH %q, got %q", decoy, got)
	}
}

func TestResolve_Missing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FESTIVAL_HOME", home)
	t.Setenv("PATH", t.TempDir())
	_, err := Resolve(context.Background(), "definitely-not-a-real-tool-xyz")
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestResolve_RejectsPath(t *testing.T) {
	_, err := Resolve(context.Background(), "../camp")
	if err == nil {
		t.Fatal("expected reject")
	}
}

func TestCatalog_NonEmpty(t *testing.T) {
	c := Catalog()
	if len(c) == 0 {
		t.Fatal("empty catalog")
	}
	for _, e := range c {
		if e.Label == "" || e.Spec.Tool == "" {
			t.Fatalf("invalid entry: %+v", e)
		}
	}
}
