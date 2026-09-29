//go:build container_fs

package cli_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
)

// Run as an unprivileged user with real Linux Camp and Fest binaries. This
// exercises download/activation and the already-current update through the CLI.
func TestStarterInstallAndCurrentUpdate(t *testing.T) {
	campPath, festPath := os.Getenv("STARTER_CAMP_BINARY"), os.Getenv("STARTER_FEST_BINARY")
	if campPath == "" || festPath == "" {
		t.Skip("set STARTER_CAMP_BINARY and STARTER_FEST_BINARY in the container lane")
	}
	if os.Geteuid() == 0 {
		t.Fatal("run starter onboarding integration as a regular user")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("CAMP_REGISTRY_PATH", "")
	t.Setenv("CAMP_ROOT", "")
	t.Setenv("FESTIVAL_HOME", filepath.Join(home, "installer"))
	t.Setenv("OBEY_INSTALLER_HOME", "")
	files := map[string]string{"festival": "#!/bin/sh\necho 0.9.99\n"}
	for name, path := range map[string]string{"camp": campPath, "fest": festPath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
	}
	tarball := buildSuiteTarGz(t, files)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(tarball) }))
	defer server.Close()
	manifest := strings.ReplaceAll(productManifest(server.URL+"/suite.tar.gz", sha256Hex(tarball)), "0.2.10", "0.9.99")
	repo := fixtureInstallMarketplaceManifest(t, manifest)
	if _, stderr, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey", "--allow-unverified"); err != nil {
		t.Fatalf("%v: %s", err, stderr)
	}
	out, stderr, err := runInstaller(t, "install", "festival", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("%v: %s", err, stderr)
	}
	var install app.InstallResult
	dataOf(t, out, &install)
	if install.Starter == nil || install.Starter.Action != "created" {
		t.Fatalf("install starter: %+v", install.Starter)
	}
	target := filepath.Join(home, "campaigns", "festival")
	if install.Starter.Path != target {
		t.Fatalf("path: %s", install.Starter.Path)
	}
	campConfig := filepath.Join(home, "config", "obey", "campaign")
	// Simulate an existing installation which predates starter onboarding.
	for _, path := range []string{target, filepath.Join(campConfig, "registry.json"), filepath.Join(campConfig, "starter.json")} {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}
	out, stderr, err = runInstaller(t, "update", "festival", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("%v: %s", err, stderr)
	}
	var update app.UpdateResult
	dataOf(t, out, &update)
	if update.Action != "current" || update.Starter == nil || update.Starter.Action != "created" {
		t.Fatalf("update: %+v starter: %+v", update, update.Starter)
	}
	// Once complete, a deliberately removed workspace stays removed on update.
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(campConfig, "registry.json")); err != nil {
		t.Fatal(err)
	}
	out, _, err = runInstaller(t, "update", "festival", "--allow-unverified", "--json")
	if err != nil {
		t.Fatal(err)
	}
	dataOf(t, out, &update)
	if update.Starter.Action != "complete" {
		t.Fatalf("%+v", update.Starter)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("deleted starter returned: %v", err)
	}
}
