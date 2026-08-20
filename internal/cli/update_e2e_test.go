package cli_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// versionedBinary is a shell-script fixture that answers `version --short`
// with a parseable version, exercising UpdateFestival's detectLiveVersion
// probe instead of leaving it unexercised. Any other invocation just echoes
// a marker so tests can also assert body content changed across upgrades.
func versionedBinary(name, version string) string {
	return fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"version\" ] && [ \"$2\" = \"--short\" ]; then\n  echo %s\n  exit 0\nfi\necho %s-%s\n", version, name, version)
}

// e2eManifest is a single-release suite manifest at the given version,
// pointing at the given archive URL/sha256. Rewriting and re-serving this
// (with a new commit to the fixture marketplace repo) simulates a new
// release being published.
func e2eManifest(version, url, sha string) string {
	return fmt.Sprintf(`{
  "schema_version": 1,
  "id": "obedience-corp/festival",
  "class": "product",
  "display_name": "Festival",
  "summary": "Camp + Fest suite",
  "provides_binaries": ["camp", "fest", "festival"],
  "releases": [
    {
      "version": %q,
      "channel": "stable",
      "published_at": "2026-06-08T00:00:00Z",
      "components": {"camp": %q, "fest": %q, "festival": %q},
      "compatibility": {"os": [%q], "arch": [%q]},
      "dependencies": [],
      "artifacts": [
        {"kind": "suite-archive", "os": %q, "arch": %q, "url": %q, "sha256": %q, "filename": "festival.tar.gz", "binaries": ["camp", "fest", "festival"]}
      ],
      "install": {"entries": [
        {"kind": "binary", "source": "camp", "executable_name": "camp"},
        {"kind": "binary", "source": "fest", "executable_name": "fest"},
        {"kind": "binary", "source": "festival", "executable_name": "festival"}
      ]}
    }
  ]
}`, version, version, version, version, runtime.GOOS, runtime.GOARCH, runtime.GOOS, runtime.GOARCH, url, sha)
}

// TestUpdateE2E_SelfReplaceAcrossVersionAndOldProcessFinishesCleanly proves
// two guarantees at once, per its name: an update from v1.0.0 to v1.1.0
// replaces all three binaries and rewrites the receipt end to end, and the
// still-running old-version process (this test binary, symlinked into the
// managed bin dir) completes reading and reporting its own full, well-formed
// result envelope after its own executable file was replaced out from under
// it. Do not simplify this to only checking the exit code: the well-formed
// envelope assertion below is the "old process finishes cleanly" property.
func TestUpdateE2E_SelfReplaceAcrossVersionAndOldProcessFinishesCleanly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	symlinkSelfAsManagedFestival(t, home)
	ctx := context.Background()

	v1 := map[string]string{
		"camp":     versionedBinary("camp", "1.0.0"),
		"fest":     versionedBinary("fest", "1.0.0"),
		"festival": versionedBinary("festival", "1.0.0"),
	}
	v1tar := buildSuiteTarGz(t, v1)
	v1sum := sha256Hex(v1tar)

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("/v1.tar.gz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(v1tar) })

	repo := fixtureInstallMarketplaceManifest(t, e2eManifest("1.0.0", srv.URL+"/v1.tar.gz", v1sum))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	installOut, installErrOut, err := runInstaller(t, "install", "festival", "--allow-unverified")
	if err != nil {
		t.Fatalf("install v1: %v\n%s\n%s", err, installOut, installErrOut)
	}

	binDir, _ := state.BinDir(ctx)
	v1Hash := func(name string) string {
		b, err := os.ReadFile(filepath.Join(binDir, name))
		if err != nil {
			t.Fatalf("read %s after v1 install: %v", name, err)
		}
		sum := sha256.Sum256(b)
		return hex.EncodeToString(sum[:])
	}
	campV1Hash := v1Hash("camp")
	festV1Hash := v1Hash("fest")
	festivalV1Hash := v1Hash("festival")

	// Publish v1.1.0: a second archive, a rewritten manifest, a second commit
	// to the same fixture marketplace repo.
	v2 := map[string]string{
		"camp":     versionedBinary("camp", "1.1.0"),
		"fest":     versionedBinary("fest", "1.1.0"),
		"festival": versionedBinary("festival", "1.1.0"),
	}
	v2tar := buildSuiteTarGz(t, v2)
	v2sum := sha256Hex(v2tar)
	mux.HandleFunc("/v2.tar.gz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(v2tar) })

	writeFile(t, filepath.Join(repo, "packages", "obedience-corp", "festival", "obey-package.json"), e2eManifest("1.1.0", srv.URL+"/v2.tar.gz", v2sum))
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "release 1.1.0")

	if _, errOut, err := runInstaller(t, "marketplace", "refresh"); err != nil {
		t.Fatalf("marketplace refresh: %v\n%s", err, errOut)
	}

	// The v1 install placed a real festival binary over the earlier symlink;
	// put the symlink back now (after capturing the v1 hashes above) so the
	// update below sees this test binary as the managed hub and self-replaces.
	symlinkSelfAsManagedFestival(t, home)

	updateOut, updateErrOut, err := runInstaller(t, "update", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update: %v\n%s\n%s", err, updateOut, updateErrOut)
	}

	// The full envelope, not just the exit code: proves the old process (this
	// test binary, still executing from its now-unlinked inode after the
	// managed bin dir's festival entry was replaced) finished writing a
	// complete, well-formed result.
	var res struct {
		Package       string `json:"package"`
		Action        string `json:"action"`
		Version       string `json:"version"`
		From          string `json:"from"`
		SelfPlacement string `json:"self_placement"`
		SelfPath      string `json:"self_path"`
		SelfReplaced  bool   `json:"self_replaced"`
	}
	dataOf(t, updateOut, &res)
	if res.Package != festivalPackageIDForTest {
		t.Fatalf("package = %q, want %q: envelope incomplete", res.Package, festivalPackageIDForTest)
	}
	if res.Action != "upgraded" {
		t.Fatalf("action = %q, want upgraded", res.Action)
	}
	if res.Version != "1.1.0" {
		t.Fatalf("version = %q, want 1.1.0", res.Version)
	}
	if res.From != "1.0.0" {
		t.Fatalf("from = %q, want 1.0.0", res.From)
	}
	if !res.SelfReplaced {
		t.Fatalf("self_replaced = false, want true: %+v", res)
	}
	if res.SelfPlacement != "managed" {
		t.Fatalf("self_placement = %q, want managed: %+v", res.SelfPlacement, res)
	}

	v2Hash := func(name string) string {
		b, err := os.ReadFile(filepath.Join(binDir, name))
		if err != nil {
			t.Fatalf("read %s after v1.1.0 update: %v", name, err)
		}
		sum := sha256.Sum256(b)
		return hex.EncodeToString(sum[:])
	}
	if got := v2Hash("camp"); got == campV1Hash {
		t.Fatal("camp hash unchanged after upgrade")
	}
	if got := v2Hash("fest"); got == festV1Hash {
		t.Fatal("fest hash unchanged after upgrade")
	}
	if got := v2Hash("festival"); got == festivalV1Hash {
		t.Fatal("festival hash unchanged after upgrade")
	}

	rec, err := receipts.Get(ctx, mustDB(t, ctx, home), festivalPackageIDForTest)
	if err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if rec.Version != "1.1.0" {
		t.Fatalf("receipt version = %q, want 1.1.0", rec.Version)
	}
	if len(rec.OwnedFiles) != 3 {
		t.Fatalf("receipt owned files = %d, want 3: %+v", len(rec.OwnedFiles), rec.OwnedFiles)
	}
	wantHash := map[string]string{
		filepath.Join(binDir, "camp"):     v2Hash("camp"),
		filepath.Join(binDir, "fest"):     v2Hash("fest"),
		filepath.Join(binDir, "festival"): v2Hash("festival"),
	}
	for _, of := range rec.OwnedFiles {
		if wantHash[of.Path] != of.Hash {
			t.Fatalf("owned file %s hash = %s, want %s (new v1.1.0 hash)", of.Path, of.Hash, wantHash[of.Path])
		}
	}
}
