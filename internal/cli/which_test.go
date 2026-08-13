package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/festival-installer/internal/cli"
)

func runWhich(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := &cobra.Command{Use: "festival", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(cli.NewWhichCommand())
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"which"}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func fakeBinary(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestWhich_FoundOnPath(t *testing.T) {
	binDir := t.TempDir()
	want := fakeBinary(t, binDir, "camp")
	t.Setenv("PATH", binDir)
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())

	out, _, err := runWhich(t, "camp")
	if err != nil {
		t.Fatalf("which camp: %v", err)
	}
	if got := strings.TrimSpace(out); got != want {
		t.Fatalf("path mismatch: got %q want %q", got, want)
	}
}

func TestWhich_ManagedMatchNotShadowed(t *testing.T) {
	home := t.TempDir()
	managedBin := filepath.Join(home, "bin")
	managed := fakeBinary(t, managedBin, "camp")
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", managedBin)

	out, errOut, err := runWhich(t, "camp", "--json")
	if err != nil {
		t.Fatalf("which camp --json: %v", err)
	}
	if !strings.Contains(out, "\"shadowed\": false") {
		t.Fatalf("expected not shadowed when PATH resolves to managed:\n%s", out)
	}
	if !strings.Contains(out, managed) {
		t.Fatalf("expected managed path %q in output:\n%s", managed, out)
	}
	if errOut != "" {
		t.Fatalf("expected no shadow warning, got: %s", errOut)
	}
}

func TestWhich_Shadowed(t *testing.T) {
	home := t.TempDir()
	managedBin := filepath.Join(home, "bin")
	fakeBinary(t, managedBin, "camp")
	otherDir := t.TempDir()
	other := fakeBinary(t, otherDir, "camp")

	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", otherDir+string(os.PathListSeparator)+managedBin)

	out, errOut, err := runWhich(t, "camp")
	if err != nil {
		t.Fatalf("which camp: %v", err)
	}
	if got := strings.TrimSpace(out); got != other {
		t.Fatalf("expected active path %q, got %q", other, got)
	}
	if !strings.Contains(errOut, "shadowed by "+other) {
		t.Fatalf("expected shadow warning on stderr, got: %s", errOut)
	}
}

func TestWhich_NotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())

	_, _, err := runWhich(t, "definitely-not-a-real-tool")
	if err == nil {
		t.Fatal("expected error for unresolvable tool")
	}
	if !strings.Contains(err.Error(), "E_TOOL_NOT_FOUND") {
		t.Fatalf("expected E_TOOL_NOT_FOUND, got: %v", err)
	}
}

func TestWhich_HomeMisconfigured(t *testing.T) {
	binDir := t.TempDir()
	fakeBinary(t, binDir, "camp")
	t.Setenv("PATH", binDir)
	t.Setenv("OBEY_INSTALLER_HOME", "relative/not/absolute")

	_, _, err := runWhich(t, "camp")
	if err == nil {
		t.Fatal("expected error when OBEY_INSTALLER_HOME is not absolute")
	}
	if !strings.Contains(err.Error(), "E_HOME_NOT_ABS") {
		t.Fatalf("expected E_HOME_NOT_ABS, got: %v", err)
	}
}

func TestWhich_ShowAll(t *testing.T) {
	home := t.TempDir()
	managedBin := filepath.Join(home, "bin")
	fakeBinary(t, managedBin, "camp")
	otherDir := t.TempDir()
	fakeBinary(t, otherDir, "camp")

	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("PATH", otherDir+string(os.PathListSeparator)+managedBin)

	out, _, err := runWhich(t, "camp", "--show-all")
	if err != nil {
		t.Fatalf("which camp --show-all: %v", err)
	}
	if !strings.Contains(out, "path") || !strings.Contains(out, "managed") {
		t.Fatalf("expected both path and managed rows:\n%s", out)
	}
	if !strings.Contains(out, "shadowed") {
		t.Fatalf("expected managed row marked shadowed:\n%s", out)
	}
}
