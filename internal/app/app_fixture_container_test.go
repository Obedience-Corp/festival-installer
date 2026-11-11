//go:build container_fs && !windows

package app

import (
	"context"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/apptest/appfixture"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
)

// appFixture wraps the shared signed catalog builder and stubs the official
// seed so no test can reach the network.
type appFixture struct {
	t         *testing.T
	Home      string               // HOME; FESTIVAL_HOME is <Home>/installer
	Repo      string               // git repo of the fixture marketplace
	Verify    source.VerifyOptions // KeyStore trusts only the fixture key
	SeedCalls int                  // calls to the stubbed ensureOfficialSeed
	cat       *appfixture.Catalog
}

type appFixtureOptions = appfixture.PublishOptions

func newAppFixture(t *testing.T) *appFixture {
	t.Helper()
	cat := appfixture.NewCatalog(t)
	f := &appFixture{t: t, Home: cat.Home, Repo: cat.Repo, Verify: cat.Verify, cat: cat}
	previous := ensureOfficialSeed
	ensureOfficialSeed = func(context.Context, source.VerifyOptions) error {
		f.SeedCalls++
		return nil
	}
	t.Cleanup(func() { ensureOfficialSeed = previous })
	return f
}

func (f *appFixture) publish(m metadata.PackageManifest, o appFixtureOptions) {
	f.t.Helper()
	f.cat.Publish(m, o)
}

func (f *appFixture) register(name string) {
	f.t.Helper()
	f.cat.Register(name)
}

func appFixtureManifest(releases ...metadata.Release) metadata.PackageManifest {
	return appfixture.Manifest(releases...)
}

func appFixtureRelease(version, channel string, arts ...metadata.Artifact) metadata.Release {
	return appfixture.Release(version, channel, arts...)
}

func appFixtureArtifact(kind, goos, goarch string) metadata.Artifact {
	return appfixture.Artifact(kind, goos, goarch)
}
