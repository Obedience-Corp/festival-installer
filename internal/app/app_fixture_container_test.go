//go:build container_fs && !windows

package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

// appFixture is a signed fixture marketplace listing the Festival App. It is
// shared by the App tasks and must only run in the container lane.
type appFixture struct {
	t         *testing.T
	Home      string               // HOME; FESTIVAL_HOME is <Home>/installer
	Repo      string               // git repo of the fixture marketplace
	Verify    source.VerifyOptions // KeyStore trusts only the fixture key
	SeedCalls int                  // calls to the stubbed ensureOfficialSeed
	private   ed25519.PrivateKey
}

type appFixtureOptions struct {
	OmitApp          bool // catalog lists no festival-app
	UnsignedCatalog  bool
	UnsignedManifest bool
	ManifestSignedBy ed25519.PrivateKey // nil: the fixture key
}

func newAppFixture(t *testing.T) *appFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "installer"))
	t.Setenv("OBEY_INSTALLER_HOME", "")
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	f := &appFixture{
		t: t, Home: home, Repo: t.TempDir(), private: private,
		Verify: source.VerifyOptions{KeyStore: verify.NewStaticStore(map[string]ed25519.PublicKey{"fixture": pub})},
	}
	browseSeedGit(t, f.Repo, "init", "-b", "main")
	previous := ensureOfficialSeed
	ensureOfficialSeed = func(context.Context, source.VerifyOptions) error {
		f.SeedCalls++
		return nil
	}
	t.Cleanup(func() { ensureOfficialSeed = previous })
	return f
}

func (f *appFixture) write(rel string, doc any, sign bool, key ed25519.PrivateKey) {
	f.t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		f.t.Fatal(err)
	}
	canonical, err := verify.CanonicalizeJSON(raw)
	if err != nil {
		f.t.Fatal(err)
	}
	path := filepath.Join(f.Repo, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, canonical, 0o644); err != nil {
		f.t.Fatal(err)
	}
	if !sign {
		_ = os.Remove(path + ".sig")
		return
	}
	sig, err := verify.MarshalDetachedSignature(verify.Signature{KeyID: "fixture", Algorithm: verify.AlgorithmEd25519, Bytes: ed25519.Sign(key, canonical)})
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path+".sig", sig, 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// publish writes the catalog and the App manifest, signs what o allows, and commits.
func (f *appFixture) publish(m metadata.PackageManifest, o appFixtureOptions) {
	f.t.Helper()
	const manifestPath = "packages/obedience-corp/festival-app/obey-package.json"
	pkgs := []source.PackageRef{}
	if !o.OmitApp {
		pkgs = append(pkgs, source.PackageRef{
			ID: AppPackageID, DisplayName: AppProductName, Class: "product", Summary: "Festival App desktop app",
			Channels: []string{"stable"}, ManifestPath: manifestPath,
		})
	}
	market := source.Marketplace{ID: "fixture/app", Name: "Festival App fixture", SchemaVersion: "1", Namespace: "obedience-corp", Packages: pkgs}
	f.write("obey-marketplace.json", market, !o.UnsignedCatalog, f.private)
	key := f.private
	if o.ManifestSignedBy != nil {
		key = o.ManifestSignedBy
	}
	if !o.OmitApp {
		f.write(manifestPath, m, !o.UnsignedManifest, key)
	}
	browseSeedGit(f.t, f.Repo, "add", "-A")
	browseSeedGit(f.t, f.Repo, "commit", "--allow-empty", "-m", "fixture "+time.Now().Format(time.RFC3339Nano))
}

func (f *appFixture) register(name string) {
	f.t.Helper()
	if _, err := source.AddMarketplace(context.Background(), f.Repo, name, f.Verify); err != nil {
		f.t.Fatal(err)
	}
}

func appFixtureManifest(releases ...metadata.Release) metadata.PackageManifest {
	return metadata.PackageManifest{
		SchemaVersion: 1, ID: AppPackageID, Class: "product", DisplayName: AppProductName,
		Summary: "Festival App desktop app", Releases: releases,
	}
}

func appFixtureRelease(version, channel string, arts ...metadata.Artifact) metadata.Release {
	return metadata.Release{
		Version: version, Channel: channel, PublishedAt: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		Compatibility: metadata.Compatibility{OS: []string{"darwin", "linux", "windows"}, Arch: []string{"amd64", "arm64"}},
		Dependencies:  []metadata.Dependency{},
		Artifacts:     arts,
		Install: metadata.Install{Entries: []metadata.InstallEntry{{
			Kind: metadata.InstallKindDesktopApp, Source: "festival-app", BundleID: AppBundleID, ProductName: AppProductName,
		}}},
	}
}

func appFixtureArtifact(kind, goos, goarch string) metadata.Artifact {
	name := "Festival.App_" + kind + "_" + goos + "_" + goarch
	return metadata.Artifact{
		Kind: kind, OS: goos, Arch: goarch,
		URL:              "https://github.com/Obedience-Corp/obey-releases/releases/download/app-v0.1.8/" + name,
		Sha256:           "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Filename:         name,
		UpdaterSignature: "c2ln",
	}
}
