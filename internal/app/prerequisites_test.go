package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestPrerequisiteCommandsUseSystemAuthorizationAndFixedArguments(t *testing.T) {
	for _, tc := range []struct {
		os, manager string
		want        []string
		pending     bool
	}{
		{"linux", "/usr/bin/apt-get", []string{"/usr/bin/pkexec", "/usr/bin/apt-get", "install", "--yes", "git"}, false},
		{"linux", "/usr/bin/pacman", []string{"/usr/bin/pkexec", "/usr/bin/pacman", "-S", "--needed", "--noconfirm", "git"}, false},
		{"darwin", "", []string{"/usr/bin/xcode-select", "--install"}, true},
		{"windows", "", []string{"winget", "install", "--id", "Git.Git", "--exact", "--source", "winget", "--accept-source-agreements", "--accept-package-agreements"}, false},
	} {
		got, pending, err := prerequisiteCommand(tc.os, func(path string) bool { return path == "/usr/bin/pkexec" || path == tc.manager })
		if err != nil || pending != tc.pending || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: %v %v %v", tc.os, got, pending, err)
		}
	}
	if _, _, err := prerequisiteCommand("linux", func(string) bool { return false }); err == nil {
		t.Fatal("missing authorization helper accepted")
	}
}

func TestGitLauncherWithoutWorkingGitIsNotReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix launcher fixture")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "git")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatal(err)
	}
	if gitReady(context.Background()) {
		t.Fatal("launcher mistaken for installed Git")
	}
	for _, p := range resolvePrerequisites(context.Background()) {
		if p.Name == "git" && p.Present {
			t.Fatal("status accepted nonworking Git")
		}
	}
}
