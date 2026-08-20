package cli_test

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

func TestVersion_Bare(t *testing.T) {
	out, _, err := runInstaller(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if strings.TrimSpace(out) != testFestivalVersion {
		t.Fatalf("bare version = %q, want %q", strings.TrimSpace(out), testFestivalVersion)
	}
}

func TestVersion_ShortAcceptedByLooksLikeVersion(t *testing.T) {
	out, _, err := runInstaller(t, "version", "--short")
	if err != nil {
		t.Fatalf("version --short: %v", err)
	}
	got := strings.TrimSpace(out)
	if got != testFestivalVersion {
		t.Fatalf("version --short = %q, want %q", got, testFestivalVersion)
	}
	if !app.LooksLikeVersion(got) {
		t.Fatalf("version --short output %q is not accepted by app.LooksLikeVersion", got)
	}
}

func TestVersion_RejectsExtraArgs(t *testing.T) {
	_, _, err := runInstaller(t, "version", "extra-arg")
	if err == nil {
		t.Fatal("expected festival version extra-arg to be rejected")
	}
}
