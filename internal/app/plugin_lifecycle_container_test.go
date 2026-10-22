//go:build container_fs && !windows

package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/release"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
	"github.com/Obedience-Corp/festival-installer/internal/verify"
)

func TestPluginProfileSignedLifecyclePreservesState(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "installer"))
	t.Setenv("OBEY_INSTALLER_HOME", "")
	tools := filepath.Join(home, "tools")
	if err := os.MkdirAll(tools, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tools, "camp"), []byte("#!/bin/sh\necho 'camp 0.9.0'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	git := t.TempDir()
	browseSeedGit(t, git, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(git, "README"), []byte("fixture release refs"), 0o644); err != nil {
		t.Fatal(err)
	}
	browseSeedGit(t, git, "add", ".")
	browseSeedGit(t, git, "commit", "-m", "fixture refs")
	browseSeedGit(t, git, "tag", "v99.0.0")
	browseSeedGit(t, git, "tag", "fest-gif-v90.0.0")
	browseSeedGit(t, git, "tag", "camp-app-v0.1.0")
	var downloads atomic.Int32
	var corrupt atomic.Bool
	payload := func(version string) []byte { return []byte("#!/bin/sh\necho 'camp-app " + version + "'\n") }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
		if len(parts) != 2 {
			http.NotFound(w, request)
			return
		}
		version := strings.TrimPrefix(parts[0], "camp-app-v")
		data := payload(version)
		if parts[1] == "checksums.txt" {
			sum := sha256.Sum256(data)
			_, _ = fmt.Fprintf(w, "%s  camp-app-%s-%s-%s\n", hex.EncodeToString(sum[:]), version, runtime.GOOS, runtime.GOARCH)
			return
		}
		downloads.Add(1)
		if corrupt.Load() {
			data = []byte("corrupt")
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	pub, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	catalog := t.TempDir()
	browseSeedGit(t, catalog, "init", "-b", "main")
	market := source.Marketplace{ID: "fixture/signed", Name: "Signed plugin fixture", SchemaVersion: "1", Namespace: "obedience-corp", Packages: []source.PackageRef{{
		ID: "obedience-corp/camp-app", DisplayName: "Camp App", Class: "plugin", HostRuntimes: []string{"camp-cli"}, Channels: []string{"stable"},
		ReleaseSource: &release.Source{Type: "git", Repo: git, TagPrefix: "camp-app-v", AssetURL: server.URL + "/{tag}/camp-app-{version}-{os}-{arch}", ChecksumsURL: server.URL + "/{tag}/checksums.txt", OS: map[string]string{runtime.GOOS: runtime.GOOS}, Arch: map[string]string{runtime.GOARCH: runtime.GOARCH}},
	}}}
	raw, err := json.Marshal(market)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := verify.CanonicalizeJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := verify.MarshalDetachedSignature(verify.Signature{KeyID: "fixture", Algorithm: verify.AlgorithmEd25519, Bytes: ed25519.Sign(private, canonical)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalog, "obey-marketplace.json"), canonical, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalog, "obey-marketplace.json.sig"), signature, 0o644); err != nil {
		t.Fatal(err)
	}
	browseSeedGit(t, catalog, "add", ".")
	browseSeedGit(t, catalog, "commit", "-m", "signed fixture catalog")
	vo := source.VerifyOptions{KeyStore: verify.NewStaticStore(map[string]ed25519.PublicKey{"fixture": pub})}
	if _, err := source.AddMarketplace(ctx, catalog, "profile-fixture", vo); err != nil {
		t.Fatal(err)
	}
	installed, err := InstallTarget(ctx, "obedience-corp/camp-app", InstallOptions{Verify: vo})
	if err != nil || installed.Version != "0.1.0" {
		t.Fatalf("initial: %+v %v", installed, err)
	}
	bin, err := state.BinDir(ctx)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bin, "camp-app")
	before := downloads.Load()
	result, _, err := UpdatePlugin(ctx, "obedience-corp/camp-app", UpdateOptions{Verify: vo})
	if err != nil || result.Action != "current" || downloads.Load() != before {
		t.Fatalf("current: %+v %v", result, err)
	}

	// Receipt corruption must not be treated as version zero and overwritten.
	owned, found, err := pluginReceipt(ctx, "obedience-corp/camp-app")
	if err != nil || !found {
		t.Fatalf("receipt: %v", err)
	}
	installerHome, err := state.Home(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db, err := state.OpenDB(ctx, installerHome)
	if err != nil {
		t.Fatal(err)
	}
	invalid := owned
	invalid.Version = "broken-version"
	if err := receipts.Write(ctx, db.Raw(), invalid); err != nil {
		t.Fatal(err)
	}
	before = downloads.Load()
	if _, _, err := UpdatePlugin(ctx, "obedience-corp/camp-app", UpdateOptions{Verify: vo}); errpkg.Code(err) != "E_PLUGIN_VERSION" {
		t.Fatalf("invalid receipt version accepted: %v", err)
	}
	if downloads.Load() != before {
		t.Fatal("invalid receipt downloaded an artifact")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(payload("0.1.0")) {
		t.Fatalf("invalid receipt changed file: %q %v", got, err)
	}
	if err := receipts.Write(ctx, db.Raw(), owned); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(ctx); err != nil {
		t.Fatal(err)
	}
	browseSeedGit(t, git, "tag", "camp-app-v0.2.0")
	result, _, err = UpdatePlugin(ctx, "obedience-corp/camp-app", UpdateOptions{Verify: vo})
	if err != nil || result.Action != "upgraded" || result.Version != "0.2.0" {
		t.Fatalf("upgrade: %+v %v", result, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	result, _, err = UpdatePlugin(ctx, "obedience-corp/camp-app", UpdateOptions{Verify: vo})
	if err != nil || result.Action != "repaired" {
		t.Fatalf("repair: %+v %v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(payload("0.2.0")) {
		t.Fatalf("readback %q %v", got, err)
	}
	browseSeedGit(t, git, "tag", "-d", "camp-app-v0.2.0")
	before = downloads.Load()
	result, _, err = UpdatePlugin(ctx, "obedience-corp/camp-app", UpdateOptions{Verify: vo})
	if err != nil || result.Action != "current" || result.Version != "0.2.0" || downloads.Load() != before {
		t.Fatalf("newer compatible: %+v %v", result, err)
	}
	browseSeedGit(t, git, "tag", "camp-app-v0.2.0")
	browseSeedGit(t, git, "tag", "camp-app-v0.3.0")
	corrupt.Store(true)
	if _, _, err = UpdatePlugin(ctx, "obedience-corp/camp-app", UpdateOptions{Verify: vo}); err == nil {
		t.Fatal("corrupt download accepted")
	}
	receipt, found, err := pluginReceipt(ctx, "obedience-corp/camp-app")
	if err != nil || !found || receipt.Version != "0.2.0" {
		t.Fatalf("corrupt changed receipt: %+v %v", receipt, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(payload("0.2.0")) {
		t.Fatalf("corrupt changed file: %q %v", got, err)
	}

	corrupt.Store(false)
	result, _, err = UpdatePlugin(ctx, "obedience-corp/camp-app", UpdateOptions{Verify: vo})
	if err != nil || result.Action != "upgraded" || result.Version != "0.3.0" {
		t.Fatalf("retry: %+v %v", result, err)
	}
	before = downloads.Load()
	result, _, err = UpdatePlugin(ctx, "obedience-corp/camp-app", UpdateOptions{Verify: vo})
	if err != nil || result.Action != "current" || downloads.Load() != before {
		t.Fatalf("retry convergence: %+v %v", result, err)
	}
	foreign := []byte("foreign replacement")
	if err := os.WriteFile(path, foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	result, _, err = UpdatePlugin(ctx, "obedience-corp/camp-app", UpdateOptions{Verify: vo})
	if err != nil || result.Action != "unmanaged" {
		t.Fatalf("foreign: %+v %v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(foreign) {
		t.Fatalf("foreign changed: %q %v", got, err)
	}
	clone, err := source.CloneDir(ctx, "profile-fixture")
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(clone, "obey-marketplace.json")
	signedBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(signedBytes), "camp-app-v", "fest-gif-v", 1)
	if err := os.WriteFile(metadataPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := source.SourcePackages(ctx, "profile-fixture", vo); err == nil {
		t.Fatal("changed producer namespace bypassed marketplace signature")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := UpdatePlugin(cancelled, "obedience-corp/camp-app", UpdateOptions{Verify: vo}); errpkg.Code(err) != "E_PLUGIN_CTX" {
		t.Fatalf("cancel: %v", err)
	}
}
