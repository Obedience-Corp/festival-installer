package cli_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const pluginMarketplaceRoot = `{
  "id": "acme/plugins",
  "name": "Acme Plugins",
  "schema_version": "1",
  "packages": [
    {"id":"acme/fest-demo","display_name":"Fest Demo","class":"plugin","host_runtimes":["fest-cli"],"targets":[{"package":"obedience-corp/festival","runtime":"fest-cli","version_constraint":">=0.4.0"}],"channels":["stable"],"manifest_path":"packages/fest-demo/obey-package.json"}
  ]
}
`

func pluginManifest(url, sha string) string {
	return fmt.Sprintf(`{
  "schema_version": 1,
  "id": "acme/fest-demo",
  "class": "plugin",
  "display_name": "Fest Demo",
  "summary": "Demo fest plugin",
  "host_runtimes": [{"runtime": "fest-cli"}],
  "targets": [{"package": "obedience-corp/festival", "runtime": "fest-cli", "version_constraint": ">=0.4.0"}],
  "releases": [
    {
      "version": "0.1.0",
      "channel": "stable",
      "published_at": "2026-06-08T00:00:00Z",
      "compatibility": {"os": [%q], "arch": [%q]},
      "dependencies": [],
      "artifacts": [{"kind": "binary", "os": %q, "arch": %q, "url": %q, "sha256": %q, "filename": "fest-demo"}],
      "install": {"entries": [{"kind": "binary", "source": "fest-demo", "executable_name": "fest-demo"}]}
    }
  ]
}`, runtime.GOOS, runtime.GOARCH, runtime.GOOS, runtime.GOARCH, url, sha)
}

func pluginManifestSourceOnly(url, sha string) string {
	return fmt.Sprintf(`{
  "schema_version": 1,
  "id": "acme/fest-demo",
  "class": "plugin",
  "display_name": "Fest Demo",
  "summary": "Demo fest plugin",
  "host_runtimes": [{"runtime": "fest-cli"}],
  "targets": [{"package": "obedience-corp/festival", "runtime": "fest-cli", "version_constraint": ">=0.4.0"}],
  "releases": [
    {
      "version": "0.1.0",
      "channel": "stable",
      "published_at": "2026-06-08T00:00:00Z",
      "compatibility": {"os": [%q], "arch": [%q]},
      "dependencies": [],
      "artifacts": [{"kind": "binary", "os": %q, "arch": %q, "url": %q, "sha256": %q, "filename": "fest-demo"}],
      "install": {"entries": [{"kind": "binary", "source": "fest-demo"}]}
    }
  ]
}`, runtime.GOOS, runtime.GOARCH, runtime.GOOS, runtime.GOARCH, url, sha)
}

func pluginManifestArchive(url, sha string) string {
	return fmt.Sprintf(`{
  "schema_version": 1,
  "id": "acme/fest-demo",
  "class": "plugin",
  "display_name": "Fest Demo",
  "summary": "Demo fest plugin",
  "host_runtimes": [{"runtime": "fest-cli"}],
  "targets": [{"package": "obedience-corp/festival", "runtime": "fest-cli", "version_constraint": ">=0.4.0"}],
  "releases": [
    {
      "version": "0.1.0",
      "channel": "stable",
      "published_at": "2026-06-08T00:00:00Z",
      "compatibility": {"os": [%q], "arch": [%q]},
      "dependencies": [],
      "artifacts": [{"kind": "tar.gz", "os": %q, "arch": %q, "url": %q, "sha256": %q, "filename": "fest-demo.tar.gz"}],
      "install": {"entries": [{"kind": "binary", "source": "fest-demo", "executable_name": "fest-demo"}]}
    }
  ]
}`, runtime.GOOS, runtime.GOARCH, runtime.GOOS, runtime.GOARCH, url, sha)
}

func fixturePluginMarketplace(t *testing.T, url, sha string) string {
	return fixturePluginMarketplaceManifest(t, pluginManifest(url, sha))
}

func gitReleaseRepo(t *testing.T, tag string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "README.md"), "plugin")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	git(t, dir, "tag", tag)
	return dir
}

func TestInstallPlugin_GitReleaseSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)

	pluginRepo := gitReleaseRepo(t, "v0.1.0")

	binBody := "#!/bin/sh\necho fest-demo\n"
	tarball := buildSuiteTarGz(t, map[string]string{"fest-demo": binBody, "README.md": "r"})
	osMap := map[string]string{"darwin": "macOS", "linux": "linux"}
	archMap := map[string]string{"amd64": "x86_64", "arm64": "arm64"}
	assetName := fmt.Sprintf("fest-demo-0.1.0-%s-%s.tar.gz", osMap[runtime.GOOS], archMap[runtime.GOARCH])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksums"):
			_, _ = fmt.Fprintf(w, "%s  %s\n", sha256Hex(tarball), assetName)
		case strings.HasSuffix(r.URL.Path, assetName):
			_, _ = w.Write(tarball)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	root := fmt.Sprintf(`{
  "id": "acme/plugins",
  "name": "Acme Plugins",
  "schema_version": "1",
  "packages": [
    {
      "id": "acme/fest-demo",
      "display_name": "Fest Demo",
      "class": "plugin",
      "host_runtimes": ["fest-cli"],
      "targets": [{"package": "obedience-corp/festival", "runtime": "fest-cli"}],
      "channels": ["stable"],
      "release_source": {
        "type": "git",
        "repo": %q,
        "asset_url": "%s/dl/fest-demo-{version}-{os}-{arch}.tar.gz",
        "checksums_url": "%s/dl/checksums"
      }
    }
  ]
}`, pluginRepo, srv.URL, srv.URL)

	mktDir := t.TempDir()
	git(t, mktDir, "init", "-b", "main")
	writeFile(t, filepath.Join(mktDir, "obey-marketplace.json"), root)
	git(t, mktDir, "add", ".")
	git(t, mktDir, "commit", "-m", "init")

	if _, errOut, err := runInstaller(t, "marketplace", "add", mktDir, "--name", "acme"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathDir, "fest"), []byte("#!/bin/sh\necho fest 0.4.5\n"), 0o755); err != nil {
		t.Fatalf("write fake fest: %v", err)
	}
	managedBin := filepath.Join(home, "bin")
	sep := string(os.PathListSeparator)
	t.Setenv("PATH", pathDir+sep+managedBin+sep+os.Getenv("PATH"))

	out, errOut, err := runInstaller(t, "install", "fest-demo", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("install fest-demo: %v\n%s", err, errOut)
	}
	var res struct {
		Version string `json:"version"`
	}
	dataOf(t, out, &res)
	if res.Version != "0.1.0" {
		t.Fatalf("expected resolved version 0.1.0 from git tag, got %q", res.Version)
	}

	landed := filepath.Join(managedBin, "fest-demo")
	got, err := os.ReadFile(landed)
	if err != nil {
		t.Fatalf("expected extracted binary at %s: %v", landed, err)
	}
	if string(got) != binBody {
		t.Fatalf("managed bin should be the extracted binary: %q", got)
	}
}

func TestInstallPlugin_TarGzArchiveExtractsBinary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)

	binBody := "#!/bin/sh\necho fest-demo\n"
	tarball := buildSuiteTarGz(t, map[string]string{
		"fest-demo":                  binBody,
		"README.md":                  "readme",
		"completions/fest-demo.bash": "comp",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	repo := fixturePluginMarketplaceManifest(t, pluginManifestArchive(srv.URL+"/fest-demo.tar.gz", sha256Hex(tarball)))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "acme"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathDir, "fest"), []byte("#!/bin/sh\necho fest 0.4.5\n"), 0o755); err != nil {
		t.Fatalf("write fake fest: %v", err)
	}
	managedBin := filepath.Join(home, "bin")
	t.Setenv("PATH", pathDir+string(os.PathListSeparator)+managedBin)

	if _, errOut, err := runInstaller(t, "install", "fest-demo", "--allow-unverified"); err != nil {
		t.Fatalf("install fest-demo: %v\n%s", err, errOut)
	}

	landed := filepath.Join(managedBin, "fest-demo")
	got, err := os.ReadFile(landed)
	if err != nil {
		t.Fatalf("expected extracted binary at %s: %v", landed, err)
	}
	if string(got) != binBody {
		t.Fatalf("managed bin should be the EXTRACTED binary, not the archive: %q", got)
	}
	fi, _ := os.Stat(landed)
	if fi.Mode()&0o111 == 0 {
		t.Fatalf("extracted binary should be executable, mode=%v", fi.Mode())
	}
}

func fixturePluginMarketplaceManifest(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), pluginMarketplaceRoot)
	writeFile(t, filepath.Join(dir, "packages", "fest-demo", "obey-package.json"), manifest)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	return dir
}

func TestInstallFestPlugin_LandsDiscoverableAndUninstalls(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)

	binBody := "#!/bin/sh\necho fest-demo\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(binBody))
	}))
	t.Cleanup(srv.Close)

	repo := fixturePluginMarketplace(t, srv.URL+"/fest-demo", sha256Hex([]byte(binBody)))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "acme"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathDir, "fest"), []byte("#!/bin/sh\necho fest 0.4.5\n"), 0o755); err != nil {
		t.Fatalf("write fake fest: %v", err)
	}
	managedBin := filepath.Join(home, "bin")
	t.Setenv("PATH", pathDir+string(os.PathListSeparator)+managedBin)

	if _, errOut, err := runInstaller(t, "install", "fest-demo", "--allow-unverified"); err != nil {
		t.Fatalf("install fest-demo: %v\n%s", err, errOut)
	}

	landed := filepath.Join(managedBin, "fest-demo")
	info, err := os.Stat(landed)
	if err != nil {
		t.Fatalf("expected %s: %v", landed, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("expected %s executable, mode=%v", landed, info.Mode())
	}
	got, _ := os.ReadFile(landed)
	if string(got) != binBody {
		t.Fatalf("installed plugin content mismatch: %q", got)
	}

	resolved, err := exec.LookPath("fest-demo")
	if err != nil {
		t.Fatalf("fest-demo not discoverable on PATH (fest scans for fest- prefix + exec bit): %v", err)
	}
	if resolvePathForTest(resolved) != resolvePathForTest(landed) {
		t.Fatalf("PATH resolves fest-demo to %s, want %s", resolved, landed)
	}

	if _, errOut, err := runInstaller(t, "uninstall", "fest-demo"); err != nil {
		t.Fatalf("uninstall fest-demo: %v\n%s", err, errOut)
	}
	if _, err := os.Stat(landed); !os.IsNotExist(err) {
		t.Fatalf("expected %s removed after uninstall", landed)
	}
}

func TestInstallFestPlugin_OmittedExecutableNameLandsAtSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)

	binBody := "#!/bin/sh\necho fest-demo\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(binBody))
	}))
	t.Cleanup(srv.Close)

	repo := fixturePluginMarketplaceManifest(t, pluginManifestSourceOnly(srv.URL+"/fest-demo", sha256Hex([]byte(binBody))))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "acme"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	pathDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathDir, "fest"), []byte("#!/bin/sh\necho fest 0.4.5\n"), 0o755); err != nil {
		t.Fatalf("write fake fest: %v", err)
	}
	managedBin := filepath.Join(home, "bin")
	t.Setenv("PATH", pathDir+string(os.PathListSeparator)+managedBin)

	if _, errOut, err := runInstaller(t, "install", "fest-demo", "--allow-unverified"); err != nil {
		t.Fatalf("install fest-demo: %v\n%s", err, errOut)
	}

	landed := filepath.Join(managedBin, "fest-demo")
	fi, err := os.Stat(landed)
	if err != nil {
		t.Fatalf("expected %s to land when executable_name is omitted: %v", landed, err)
	}
	if fi.IsDir() {
		t.Fatalf("%s should be the binary, not a directory", landed)
	}
	got, _ := os.ReadFile(landed)
	if string(got) != binBody {
		t.Fatalf("installed plugin content mismatch: %q", got)
	}
}

func resolvePathForTest(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

func TestInstallPlugin_HostAbsentFails(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	t.Cleanup(srv.Close)
	repo := fixturePluginMarketplace(t, srv.URL+"/fest-demo", sha256Hex([]byte("x")))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "acme"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	t.Setenv("PATH", filepath.Join(home, "bin"))
	if _, _, err := runInstaller(t, "install", "fest-demo", "--allow-unverified"); err == nil {
		t.Fatal("expected install to fail when fest host is absent")
	}
	if _, statErr := os.Stat(filepath.Join(home, "bin", "fest-demo")); !os.IsNotExist(statErr) {
		t.Fatal("plugin should not be installed when the host is absent")
	}
}
