package app

import (
	"context"
	"errors"
	"testing"
)

func TestInstallFestivalBootstrapsOfficialSourceOnFreshHome(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")

	want := errors.New("seed unavailable")
	called := false
	previous := ensureOfficialSeed
	ensureOfficialSeed = func(context.Context) error {
		called = true
		return want
	}
	t.Cleanup(func() { ensureOfficialSeed = previous })

	_, err := InstallFestival(context.Background(), InstallOptions{})
	if !called {
		t.Fatal("InstallFestival did not bootstrap the official source")
	}
	if !errors.Is(err, want) {
		t.Fatalf("expected seed error, got %v", err)
	}
}

func TestMarketplaceListSurfacesSeedWarning(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", t.TempDir())
	t.Setenv("OBEY_INSTALLER_HOME", "")

	want := errors.New("offline")
	previous := ensureOfficialSeed
	ensureOfficialSeed = func(context.Context) error { return want }
	t.Cleanup(func() { ensureOfficialSeed = previous })

	views, err := MarketplaceList(context.Background())
	if len(views) != 0 {
		t.Fatalf("expected no sources, got %+v", views)
	}
	var warning *MarketplaceSeedWarning
	if !errors.As(err, &warning) {
		t.Fatalf("expected MarketplaceSeedWarning, got %v", err)
	}
	if !errors.Is(err, want) {
		t.Fatalf("warning lost seed error: %v", err)
	}
}
