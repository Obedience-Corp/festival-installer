package app

import (
	"reflect"
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
