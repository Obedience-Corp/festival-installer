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

func TestVersion_ShortParsesThroughTheSharedParser(t *testing.T) {
	out, _, err := runInstaller(t, "version", "--short")
	if err != nil {
		t.Fatalf("version --short: %v", err)
	}
	got := strings.TrimSpace(out)
	if got != testFestivalVersion {
		t.Fatalf("version --short = %q, want %q", got, testFestivalVersion)
	}
	parsed := app.ParseToolVersion("festival", out)
	if parsed != strings.TrimPrefix(testFestivalVersion, "v") {
		t.Fatalf("the probe reads %q out of the hub's own %q, want %q",
			parsed, got, strings.TrimPrefix(testFestivalVersion, "v"))
	}
	if !app.LooksLikeVersion(parsed) {
		t.Fatalf("parsed version %q is not accepted by app.LooksLikeVersion", parsed)
	}
}

func TestVersion_RejectsExtraArgs(t *testing.T) {
	_, _, err := runInstaller(t, "version", "extra-arg")
	if err == nil {
		t.Fatal("expected festival version extra-arg to be rejected")
	}
}
