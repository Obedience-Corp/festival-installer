package cli_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/obey-installer/internal/cli"
	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

func hasErrorCode(err error, code string) bool {
	for err != nil {
		var e *errpkg.Error
		if !errors.As(err, &e) {
			return false
		}
		if e.Code == code {
			return true
		}
		err = e.Unwrap()
	}
	return false
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func dataOf(t *testing.T, out string, target any) {
	t.Helper()
	var env struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	if !env.OK {
		t.Fatalf("envelope not ok: %s", out)
	}
	if err := json.Unmarshal(env.Data, target); err != nil {
		t.Fatalf("decode data: %v\n%s", err, out)
	}
}

func buildSuiteTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz close: %v", err)
	}
	return buf.Bytes()
}

func productManifest(url, sha string) string {
	return fmt.Sprintf(`{
  "schema_version": 1,
  "id": "obedience-corp/festival",
  "class": "product",
  "display_name": "Festival",
  "summary": "Camp + Fest suite",
  "provides_binaries": ["camp", "fest"],
  "releases": [
    {
      "version": "0.2.10",
      "channel": "stable",
      "published_at": "2026-06-08T00:00:00Z",
      "components": {"camp": "0.2.11", "fest": "0.4.5"},
      "compatibility": {"os": [%q], "arch": [%q]},
      "dependencies": [],
      "artifacts": [
        {"kind": "suite-archive", "os": %q, "arch": %q, "url": %q, "sha256": %q, "filename": "festival.tar.gz", "binaries": ["camp", "fest"]}
      ],
      "install": {"entries": [
        {"kind": "binary", "source": "camp", "executable_name": "camp"},
        {"kind": "binary", "source": "fest", "executable_name": "fest"}
      ]}
    }
  ]
}`, runtime.GOOS, runtime.GOARCH, runtime.GOOS, runtime.GOARCH, url, sha)
}

const installMarketplaceRoot = `{
  "id": "obedience-corp/official",
  "name": "Official",
  "schema_version": "1",
  "packages": [
    {"id": "obedience-corp/festival", "display_name": "Festival", "class": "product", "manifest_path": "packages/obedience-corp/festival/obey-package.json"}
  ]
}
`

func productManifestUnsafeEntry(url, sha, source, execName string) string {
	return fmt.Sprintf(`{
  "schema_version": 1,
  "id": "obedience-corp/festival",
  "class": "product",
  "display_name": "Festival",
  "summary": "Camp + Fest suite",
  "provides_binaries": ["camp"],
  "releases": [
    {
      "version": "0.2.10",
      "channel": "stable",
      "published_at": "2026-06-08T00:00:00Z",
      "components": {"camp": "0.2.11"},
      "compatibility": {"os": [%q], "arch": [%q]},
      "dependencies": [],
      "artifacts": [
        {"kind": "suite-archive", "os": %q, "arch": %q, "url": %q, "sha256": %q, "filename": "festival.tar.gz", "binaries": ["camp"]}
      ],
      "install": {"entries": [
        {"kind": "binary", "source": %q, "executable_name": %q}
      ]}
    }
  ]
}`, runtime.GOOS, runtime.GOARCH, runtime.GOOS, runtime.GOARCH, url, sha, source, execName)
}

func fixtureInstallMarketplace(t *testing.T, artifactURL, artifactSha string) string {
	return fixtureInstallMarketplaceManifest(t, productManifest(artifactURL, artifactSha))
}

func fixtureInstallMarketplaceManifest(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), installMarketplaceRoot)
	writeFile(t, filepath.Join(dir, "packages", "obedience-corp", "festival", "obey-package.json"), manifest)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	return dir
}

func runInstaller(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := &cobra.Command{Use: "obey-installer", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(cli.NewMarketplaceCommand())
	root.AddCommand(cli.NewInstallCommand())
	root.AddCommand(cli.NewUpdateCommand())
	root.AddCommand(cli.NewUninstallCommand())
	root.AddCommand(cli.NewShellInitCommand())
	root.AddCommand(cli.NewBrowseCommand())
	root.AddCommand(cli.NewDoctorCommand())
	root.AddCommand(cli.NewWhichCommand())
	cli.WrapJSONErrors(root)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestInstallFestival_E2E(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()

	campBody := "#!/bin/sh\necho camp\n"
	festBody := "#!/bin/sh\necho fest\n"
	tarball := buildSuiteTarGz(t, map[string]string{"camp": campBody, "fest": festBody})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	repo := fixtureInstallMarketplace(t, srv.URL+"/festival.tar.gz", sha256Hex(tarball))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	out, _, err := runInstaller(t, "install", "festival", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	var res struct {
		Package string   `json:"package"`
		Version string   `json:"version"`
		Channel string   `json:"channel"`
		Files   []string `json:"files"`
	}
	dataOf(t, out, &res)
	if res.Version != "0.2.10" || res.Channel != "stable" || len(res.Files) != 2 {
		t.Fatalf("unexpected install result: %+v", res)
	}

	binDir, _ := state.BinDir(ctx)
	for name, body := range map[string]string{"camp": campBody, "fest": festBody} {
		p := filepath.Join(binDir, name)
		fi, statErr := os.Stat(p)
		if statErr != nil {
			t.Fatalf("%s not installed: %v", name, statErr)
		}
		if fi.Mode().Perm() != 0o755 {
			t.Fatalf("%s mode = %o, want 0755", name, fi.Mode().Perm())
		}
		got, rErr := os.ReadFile(p)
		if rErr != nil || string(got) != body {
			t.Fatalf("%s content mismatch: %q (err %v)", name, got, rErr)
		}
	}

	rec, err := receipts.Get(ctx, mustDB(t, ctx, home), festivalPackageIDForTest)
	if err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if rec.Version != "0.2.10" || rec.Channel != "stable" || len(rec.OwnedFiles) != 2 {
		t.Fatalf("receipt wrong: %+v", rec)
	}
	wantHash := map[string]string{
		filepath.Join(binDir, "camp"): sha256Hex([]byte(campBody)),
		filepath.Join(binDir, "fest"): sha256Hex([]byte(festBody)),
	}
	for _, of := range rec.OwnedFiles {
		if wantHash[of.Path] != of.Hash {
			t.Fatalf("owned file %s hash = %s, want %s", of.Path, of.Hash, wantHash[of.Path])
		}
	}
}

func TestInstallFestival_ChecksumMismatchRollsBack(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()

	tarball := buildSuiteTarGz(t, map[string]string{"camp": "camp", "fest": "fest"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	wrongSha := sha256Hex([]byte("not-the-tarball"))
	repo := fixtureInstallMarketplace(t, srv.URL+"/festival.tar.gz", wrongSha)
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	if _, _, err := runInstaller(t, "install", "festival", "--allow-unverified"); err == nil {
		t.Fatal("expected install to fail on sha256 mismatch")
	}

	binDir, _ := state.BinDir(ctx)
	if _, err := os.Stat(filepath.Join(binDir, "camp")); !os.IsNotExist(err) {
		t.Fatalf("camp should not be installed after rollback, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "fest")); !os.IsNotExist(err) {
		t.Fatalf("fest should not be installed after rollback, stat err=%v", err)
	}
	if _, err := receipts.Get(ctx, mustDB(t, ctx, home), festivalPackageIDForTest); err == nil {
		t.Fatal("no receipt should exist after a failed install")
	}
}

func TestInstallFestival_RejectsUnsafeInstallEntry(t *testing.T) {
	cases := map[string]struct{ source, execName string }{
		"source escapes extract dir":    {source: "../escape", execName: "camp"},
		"executable name not a segment": {source: "camp", execName: "../evil"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("OBEY_INSTALLER_HOME", home)
			ctx := context.Background()

			tarball := buildSuiteTarGz(t, map[string]string{"camp": "camp"})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write(tarball)
			}))
			t.Cleanup(srv.Close)

			repo := fixtureInstallMarketplaceManifest(t, productManifestUnsafeEntry(srv.URL+"/festival.tar.gz", sha256Hex(tarball), tc.source, tc.execName))
			if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
				t.Fatalf("marketplace add: %v\n%s", err, errOut)
			}

			if _, _, err := runInstaller(t, "install", "festival", "--allow-unverified"); err == nil {
				t.Fatal("expected install to reject an unsafe install entry")
			}
			binDir, _ := state.BinDir(ctx)
			if _, err := os.Stat(filepath.Join(binDir, "camp")); !os.IsNotExist(err) {
				t.Fatalf("camp should not be installed after rejection, stat err=%v", err)
			}
		})
	}
}

func TestInstall_InvalidChannelAndTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	if _, _, err := runInstaller(t, "install", "festival", "--channel", "bogus"); err == nil {
		t.Fatal("expected error for invalid channel")
	}
	if _, _, err := runInstaller(t, "install", "widget"); err == nil {
		t.Fatal("expected error for unknown target")
	}
}

const festivalPackageIDForTest = "obedience-corp/festival"

func mustDB(t *testing.T, ctx context.Context, home string) *sql.DB {
	t.Helper()
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return db.Raw()
}

func TestInstallFestival_UnsignedRefusedWithoutAllowUnverified(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("FESTIVAL_HOME", "")

	campBody := "#!/bin/sh\necho camp\n"
	festBody := "#!/bin/sh\necho fest\n"
	tarball := buildSuiteTarGz(t, map[string]string{"camp": campBody, "fest": festBody})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	repo := fixtureInstallMarketplace(t, srv.URL+"/festival.tar.gz", sha256Hex(tarball))
	if _, errOut, err := runInstaller(t, "marketplace", "add", repo, "--name", "official-obey"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	// VER-01: unsigned manifest must refuse by default.
	_, errOut, err := runInstaller(t, "install", "festival", "--json")
	if err == nil {
		t.Fatal("expected install of unsigned package to fail without --allow-unverified")
	}
	if !hasErrorCode(err, "E_UNVERIFIED_REFUSED") {
		// Fallback: CLI may only surface the code on stderr for some wrap paths.
		if !strings.Contains(err.Error(), "E_UNVERIFIED_REFUSED") && !strings.Contains(errOut, "E_UNVERIFIED_REFUSED") {
			t.Fatalf("expected E_UNVERIFIED_REFUSED, got err=%v errOut=%s", err, errOut)
		}
	}
}
