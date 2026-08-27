package app

import (
	"context"
	"errors"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

func stubPackageManifestLoad(t *testing.T, refresh func(context.Context, string, source.VerifyOptions) ([]source.RefreshView, error), load func(context.Context, string, string, source.VerifyOptions) (metadata.PackageManifest, error)) {
	t.Helper()
	previousRefresh := refreshMarketplacesForLoad
	previousLoad := loadPackageManifest
	refreshMarketplacesForLoad = refresh
	loadPackageManifest = load
	t.Cleanup(func() {
		refreshMarketplacesForLoad = previousRefresh
		loadPackageManifest = previousLoad
	})
}

func TestLoadFreshPackageManifestRefreshesBeforeLoad(t *testing.T) {
	ctx := context.Background()
	loaded := false
	stubPackageManifestLoad(t,
		func(got context.Context, name string, _ source.VerifyOptions) ([]source.RefreshView, error) {
			if got != ctx || name != "custom-market" {
				t.Fatalf("refresh got context %v and source %q", got, name)
			}
			return []source.RefreshView{{Name: name, Changed: true}}, nil
		},
		func(got context.Context, name, packageID string, _ source.VerifyOptions) (metadata.PackageManifest, error) {
			loaded = true
			if got != ctx || name != "custom-market" || packageID != FestivalPackageID {
				t.Fatalf("load got context %v, source %q, package %q", got, name, packageID)
			}
			return metadata.PackageManifest{ID: packageID}, nil
		},
	)

	manifest, err := loadFreshPackageManifest(ctx, "custom-market", FestivalPackageID, source.VerifyOptions{})
	if err != nil {
		t.Fatalf("loadFreshPackageManifest: %v", err)
	}
	if !loaded || manifest.ID != FestivalPackageID {
		t.Fatalf("loaded = %v, manifest = %+v", loaded, manifest)
	}
}

func TestLoadFreshPackageManifestStopsOnRefreshFailure(t *testing.T) {
	want := errors.New("offline")
	loaded := false
	stubPackageManifestLoad(t,
		func(context.Context, string, source.VerifyOptions) ([]source.RefreshView, error) {
			return nil, want
		},
		func(context.Context, string, string, source.VerifyOptions) (metadata.PackageManifest, error) {
			loaded = true
			return metadata.PackageManifest{}, nil
		},
	)

	_, err := loadFreshPackageManifest(context.Background(), "official-obey", FestivalPackageID, source.VerifyOptions{})
	if !errors.Is(err, want) || errpkg.Code(err) != "E_MARKETPLACE_REFRESH" {
		t.Fatalf("expected coded refresh error wrapping %v, got %v", want, err)
	}
	if loaded {
		t.Fatal("loaded stale manifest after refresh failure")
	}
}

func TestLoadFreshPackageManifestStopsOnPerSourceFailure(t *testing.T) {
	loaded := false
	stubPackageManifestLoad(t,
		func(context.Context, string, source.VerifyOptions) ([]source.RefreshView, error) {
			return []source.RefreshView{{Name: "official-obey", Err: "signature invalid"}}, nil
		},
		func(context.Context, string, string, source.VerifyOptions) (metadata.PackageManifest, error) {
			loaded = true
			return metadata.PackageManifest{}, nil
		},
	)

	_, err := loadFreshPackageManifest(context.Background(), "official-obey", FestivalPackageID, source.VerifyOptions{})
	if errpkg.Code(err) != "E_MARKETPLACE_REFRESH" {
		t.Fatalf("expected coded refresh error, got %v", err)
	}
	if loaded {
		t.Fatal("loaded stale manifest after per-source refresh failure")
	}
}
