//go:build container_fs || app_native

// Package appfixture builds a signed fixture marketplace that lists Festival
// App. It is imported only by tests, and lives apart from internal/apptest
// because it needs internal/source, which imports internal/release, which
// imports internal/installer: internal/installer's own tests import apptest,
// so putting this builder there would be an import cycle.
package appfixture

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

const (
	packageID    = "obedience-corp/festival-app"
	bundleID     = "com.obediencecorp.festival-app"
	productName  = "Festival App"
	manifestPath = "packages/obedience-corp/festival-app/obey-package.json"
)

// Catalog is a git repo holding a signed marketplace with Festival App.
type Catalog struct {
	t       testing.TB
	Home    string               // set as HOME; see NewCatalog and NewCatalogAt for FESTIVAL_HOME
	Repo    string               // git repo of the fixture marketplace
	Verify  source.VerifyOptions // KeyStore holds the fixture key "fixture"
	Public  ed25519.PublicKey
	private ed25519.PrivateKey
}

type PublishOptions struct {
	OmitApp, UnsignedCatalog, UnsignedManifest bool
	ManifestSignedBy                           ed25519.PrivateKey // nil: the fixture key
}

// NewCatalog makes a fresh home under t.TempDir(), sets HOME=<home>,
// FESTIVAL_HOME=<home>/installer and OBEY_INSTALLER_HOME="", and creates the
// fixture repo under t.TempDir().
func NewCatalog(t testing.TB) *Catalog {
	t.Helper()
	c := NewCatalogAt(t, t.TempDir())
	t.Setenv("FESTIVAL_HOME", filepath.Join(c.Home, "installer"))
	return c
}

// NewCatalogAt uses an existing absolute home. It sets HOME=home and sets both
// FESTIVAL_HOME and OBEY_INSTALLER_HOME to "", so state.Home resolves to
// <home>/.obey/installer: the installer home a release binary run by the same
// user reads. The fixture repo goes under t.TempDir().
func NewCatalogAt(t testing.TB, home string) *Catalog {
	t.Helper()
	if !filepath.IsAbs(home) {
		t.Fatalf("appfixture: home %q is not absolute", home)
	}
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", "")
	t.Setenv("OBEY_INSTALLER_HOME", "")
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	c := &Catalog{
		t: t, Home: home, Repo: t.TempDir(), Public: pub, private: private,
		Verify: source.VerifyOptions{KeyStore: verify.NewStaticStore(map[string]ed25519.PublicKey{"fixture": pub})},
	}
	c.git("init", "-b", "main")
	return c
}

func (c *Catalog) git(args ...string) {
	c.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid"}, args...)...)
	cmd.Dir = c.Repo
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		c.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (c *Catalog) write(rel string, doc any, sign bool, key ed25519.PrivateKey) {
	c.t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		c.t.Fatal(err)
	}
	canonical, err := verify.CanonicalizeJSON(raw)
	if err != nil {
		c.t.Fatal(err)
	}
	path := filepath.Join(c.Repo, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		c.t.Fatal(err)
	}
	if err := os.WriteFile(path, canonical, 0o644); err != nil {
		c.t.Fatal(err)
	}
	if !sign {
		_ = os.Remove(path + ".sig")
		return
	}
	sig, err := verify.MarshalDetachedSignature(verify.Signature{KeyID: "fixture", Algorithm: verify.AlgorithmEd25519, Bytes: ed25519.Sign(key, canonical)})
	if err != nil {
		c.t.Fatal(err)
	}
	if err := os.WriteFile(path+".sig", sig, 0o644); err != nil {
		c.t.Fatal(err)
	}
}

// Publish writes the catalog and the App manifest, signs what o allows, and commits.
func (c *Catalog) Publish(m metadata.PackageManifest, o PublishOptions) {
	c.t.Helper()
	pkgs := []source.PackageRef{}
	if !o.OmitApp {
		pkgs = append(pkgs, source.PackageRef{
			ID: packageID, DisplayName: productName, Class: "product", Summary: "Festival App desktop app",
			Channels: []string{"stable"}, ManifestPath: manifestPath,
		})
	}
	market := source.Marketplace{ID: "fixture/app", Name: "Festival App fixture", SchemaVersion: "1", Namespace: "obedience-corp", Packages: pkgs}
	c.write("obey-marketplace.json", market, !o.UnsignedCatalog, c.private)
	key := c.private
	if o.ManifestSignedBy != nil {
		key = o.ManifestSignedBy
	}
	if !o.OmitApp {
		c.write(manifestPath, m, !o.UnsignedManifest, key)
	}
	c.git("add", "-A")
	c.git("commit", "--allow-empty", "-m", "fixture "+time.Now().Format(time.RFC3339Nano))
}

// Register adds the fixture repo as a marketplace source called name.
func (c *Catalog) Register(name string) {
	c.t.Helper()
	if _, err := source.AddMarketplace(context.Background(), c.Repo, name, c.Verify); err != nil {
		c.t.Fatal(err)
	}
}

func Manifest(releases ...metadata.Release) metadata.PackageManifest {
	return metadata.PackageManifest{
		SchemaVersion: 1, ID: packageID, Class: "product", DisplayName: productName,
		Summary: "Festival App desktop app", Releases: releases,
	}
}

// Release includes the desktop_app install entry.
func Release(version, channel string, arts ...metadata.Artifact) metadata.Release {
	return metadata.Release{
		Version: version, Channel: channel, PublishedAt: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
		Compatibility: metadata.Compatibility{OS: []string{"darwin", "linux", "windows"}, Arch: []string{"amd64", "arm64"}},
		Dependencies:  []metadata.Dependency{},
		Artifacts:     arts,
		Install: metadata.Install{Entries: []metadata.InstallEntry{{
			Kind: metadata.InstallKindDesktopApp, Source: "festival-app", BundleID: bundleID, ProductName: productName,
		}}},
	}
}

// Artifact has an https URL, a fake sha and updater_signature "c2ln"; callers
// that download it overwrite URL, Sha256 and UpdaterSignature.
func Artifact(kind, goos, goarch string) metadata.Artifact {
	name := "Festival.App_" + kind + "_" + goos + "_" + goarch
	return metadata.Artifact{
		Kind: kind, OS: goos, Arch: goarch,
		URL:              "https://github.com/Obedience-Corp/obey-releases/releases/download/app-v0.1.8/" + name,
		Sha256:           "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Filename:         name,
		UpdaterSignature: "c2ln",
	}
}
