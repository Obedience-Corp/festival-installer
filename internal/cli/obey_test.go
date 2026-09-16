package cli_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

func fixtureObeyMarketplace(t *testing.T, repoPath, baseURL string) string {
	t.Helper()
	root := fmt.Sprintf(`{
  "id": "obedience-corp/official",
  "name": "Official",
  "schema_version": "1",
  "packages": [
    {
      "id": "obedience-corp/obey",
      "display_name": "Obey Daemon",
      "class": "product",
      "host_runtimes": ["obey"],
      "channels": ["stable"],
      "release_source": {
        "type": "git",
        "repo": %q,
        "asset_url": "%s/dl/obey-{version}-{os}-{arch}.tar.gz",
        "checksums_url": "%s/dl/checksums.txt",
        "binaries": ["obey", "ob"]
      }
    }
  ]
}
`, repoPath, baseURL, baseURL)

	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), root)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	return dir
}

// obeyAssetName mirrors the default token maps in internal/release/git.go.
func obeyAssetName(version string) string {
	osMap := map[string]string{"darwin": "macOS", "linux": "linux"}
	archMap := map[string]string{"amd64": "x86_64", "arm64": "arm64"}
	return fmt.Sprintf("obey-%s-%s-%s.tar.gz", version, osMap[runtime.GOOS], archMap[runtime.GOARCH])
}

// obeyReleaseHost serves the archive and the checksums line for every version
// published during a test. The handler keys on the asset name so one server can
// answer for 0.2.0 and 0.2.1 in the same run.
type obeyReleaseHost struct {
	mu            sync.Mutex
	assets        map[string][]byte
	assetRequests int
	srv           *httptest.Server
}

func newObeyReleaseHost(t *testing.T) *obeyReleaseHost {
	t.Helper()
	h := &obeyReleaseHost{assets: map[string][]byte{}}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/checksums.txt") {
			for name, body := range h.assets {
				_, _ = fmt.Fprintf(w, "%s  %s\n", sha256Hex(body), name)
			}
			return
		}
		h.assetRequests++
		for name, body := range h.assets {
			if strings.HasSuffix(r.URL.Path, name) {
				_, _ = w.Write(body)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *obeyReleaseHost) publish(version string, tarball []byte) {
	h.publishNamed(obeyAssetName(version), tarball)
}

// publishNamed serves body under an exact asset name, for the cases where the
// name itself is the point: an artifact that is not an archive is recognised by
// what the release_source template resolves to.
func (h *obeyReleaseHost) publishNamed(name string, body []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.assets[name] = body
}

// assetDownloads counts requests for anything but the checksums file, which is
// how a test proves an artifact was never fetched.
func (h *obeyReleaseHost) assetDownloads() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.assetRequests
}

func (h *obeyReleaseHost) url() string { return h.srv.URL }

const obeyBodyPrefix = "#!/bin/sh\necho obey "
const obBodyPrefix = "#!/bin/sh\necho ob "

func obeyTarball(t *testing.T, version string) []byte {
	t.Helper()
	return buildSuiteTarGz(t, map[string]string{
		"obey":      obeyBodyPrefix + version + "\n",
		"ob":        obBodyPrefix + version + "\n",
		"README.md": "obey " + version,
	})
}

// obeyFixture sets up an isolated installer home, an isolated user home, a
// tagged release repository, a release server, and a registered marketplace.
// It returns the installer home, the release repository, and the release host so
// a caller can retag and republish for the update cases.
func obeyFixture(t *testing.T, version string) (string, string, *obeyReleaseHost) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("FESTIVAL_HOME", "")
	t.Setenv("HOME", t.TempDir())

	releaseRepo := gitReleaseRepo(t, "v"+version)
	host := newObeyReleaseHost(t)
	host.publish(version, obeyTarball(t, version))

	mkt := fixtureObeyMarketplace(t, releaseRepo, host.url())
	if _, errOut, err := runInstaller(t, "marketplace", "add", mkt, "--name", "official-obey", "--allow-unverified"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}
	return home, releaseRepo, host
}

type obeyInstallData struct {
	Package string   `json:"package"`
	Version string   `json:"version"`
	Channel string   `json:"channel"`
	Source  string   `json:"source"`
	Files   []string `json:"files"`
}

func TestInstallObey_PlacesBothBinariesUnderOneReceipt(t *testing.T) {
	ctx := context.Background()
	home, _, _ := obeyFixture(t, "0.2.0")

	out, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}

	var res obeyInstallData
	dataOf(t, out, &res)
	if res.Package != app.ObeyPackageID {
		t.Fatalf("package = %q, want %q", res.Package, app.ObeyPackageID)
	}
	if res.Version != "0.2.0" || res.Channel != "stable" {
		t.Fatalf("unexpected install result: %+v", res)
	}
	if len(res.Files) != 2 {
		t.Fatalf("expected exactly two placed files, got %v", res.Files)
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	wantBodies := map[string]string{
		"obey": obeyBodyPrefix + "0.2.0\n",
		"ob":   obBodyPrefix + "0.2.0\n",
	}
	for name, body := range wantBodies {
		p := filepath.Join(binDir, name)
		fi, statErr := os.Stat(p)
		if statErr != nil {
			t.Fatalf("%s not installed: %v", name, statErr)
		}
		if fi.Mode().Perm() != 0o755 {
			t.Fatalf("%s mode = %o, want 0755", name, fi.Mode().Perm())
		}
		got, readErr := os.ReadFile(p)
		if readErr != nil || string(got) != body {
			t.Fatalf("%s content = %q (err %v), want %q", name, got, readErr, body)
		}
	}

	rec, err := receipts.Get(ctx, mustDB(t, ctx, home), app.ObeyPackageID)
	if err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if rec.Version != "0.2.0" || rec.Channel != "stable" || len(rec.OwnedFiles) != 2 {
		t.Fatalf("receipt wrong: %+v", rec)
	}
	for _, of := range rec.OwnedFiles {
		body, ok := wantBodies[filepath.Base(of.Path)]
		if !ok {
			t.Fatalf("receipt owns an unexpected file: %s", of.Path)
		}
		if of.Hash != sha256Hex([]byte(body)) {
			t.Fatalf("owned file %s hash = %s, want %s", of.Path, of.Hash, sha256Hex([]byte(body)))
		}
		if of.Mode.Perm() != 0o755 {
			t.Fatalf("owned file %s mode = %o, want 0755", of.Path, of.Mode.Perm())
		}
	}

	for _, key := range []string{"self_placement", "self_path", "self_skipped"} {
		if strings.Contains(out, `"`+key+`"`) {
			t.Fatalf("obey payload must not carry %s:\n%s", key, out)
		}
	}
}

func TestInstallObey_MissingDeclaredBinaryRollsBack(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("FESTIVAL_HOME", "")
	t.Setenv("HOME", t.TempDir())

	releaseRepo := gitReleaseRepo(t, "v0.2.0")
	host := newObeyReleaseHost(t)
	host.publish("0.2.0", buildSuiteTarGz(t, map[string]string{"obey": obeyBodyPrefix + "0.2.0\n"}))

	mkt := fixtureObeyMarketplace(t, releaseRepo, host.url())
	if _, errOut, err := runInstaller(t, "marketplace", "add", mkt, "--name", "official-obey", "--allow-unverified"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	_, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json")
	if err == nil {
		t.Fatal("expected install to fail when a declared binary is absent from the archive")
	}
	if !hasErrorCode(err, "E_PRODUCT_BINARY_MISSING") && !strings.Contains(errOut, "E_PRODUCT_BINARY_MISSING") {
		t.Fatalf("expected E_PRODUCT_BINARY_MISSING, got err=%v errOut=%s", err, errOut)
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	for _, name := range []string{"obey", "ob"} {
		if _, statErr := os.Stat(filepath.Join(binDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("%s must not be placed when the install fails, stat err=%v", name, statErr)
		}
	}
	if _, err := receipts.Get(ctx, mustDB(t, ctx, home), app.ObeyPackageID); err == nil {
		t.Fatal("no obey receipt should exist after a failed install")
	}
}

// packageOriginPrefix builds a fake package-manager suite prefix: camp, fest,
// and festival beside a share/festival/shell helper, which is what
// classifyPath keys on to report OriginPackage. Each fake binary reports the
// given version in the shape its real counterpart does, so DetectSuite reads
// the version it would read on a real package machine.
func packageOriginPrefix(t *testing.T, version string) string {
	t.Helper()
	prefix := t.TempDir()
	binDir := filepath.Join(prefix, "bin")
	for tool, script := range map[string]string{
		"camp": suiteVersionScript("camp", "v"+version),
		"fest": suiteVersionScript("fest", "v"+version),
		// The hub prints its bare ldflags stamp, not a prefixed line.
		"festival": festivalVersionScript("v" + version),
	} {
		writeFile(t, filepath.Join(binDir, tool), script)
		if err := os.Chmod(filepath.Join(binDir, tool), 0o755); err != nil {
			t.Fatalf("chmod fake %s: %v", tool, err)
		}
	}
	writeFile(t, filepath.Join(prefix, "share", "festival", "shell", "festival.zsh"), "# helper\n")
	return binDir
}

func TestInstallObey_NotRefusedOnPackageOriginMachine(t *testing.T) {
	ctx := context.Background()
	home, _, _ := obeyFixture(t, "0.2.0")

	pkgBin := packageOriginPrefix(t, "0.3.10")
	t.Setenv("PATH", pkgBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	origin, err := app.DetectSuite(ctx)
	if err != nil {
		t.Fatalf("DetectSuite: %v", err)
	}
	if origin.Kind != app.OriginPackage {
		t.Fatalf("fixture did not classify as a package origin: %+v", origin)
	}

	// The same machine must still refuse a suite install, which is what makes
	// the obey result below meaningful rather than an accident of the fixture.
	if _, _, suiteErr := runInstaller(t, "install", "festival", "--allow-unverified", "--json"); suiteErr == nil {
		t.Fatal("expected the suite install to be refused on a package-origin machine")
	} else if !hasErrorCode(suiteErr, "E_INSTALL_PACKAGE_CHANNEL") {
		t.Fatalf("expected E_INSTALL_PACKAGE_CHANNEL for the suite, got %v", suiteErr)
	}

	out, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("obey install must not be refused on a package-origin machine: %v\n%s", err, errOut)
	}
	var res obeyInstallData
	dataOf(t, out, &res)
	if len(res.Files) != 2 {
		t.Fatalf("expected both obey binaries on a package-origin machine, got %v", res.Files)
	}
	if _, err := receipts.Get(ctx, mustDB(t, ctx, home), app.ObeyPackageID); err != nil {
		t.Fatalf("obey receipt: %v", err)
	}
}

type obeyUpdateData struct {
	Package      string `json:"package"`
	Action       string `json:"action"`
	Version      string `json:"version"`
	From         string `json:"from"`
	SelfReplaced bool   `json:"self_replaced"`
}

func TestUpdateObey_CurrentThenUpgraded(t *testing.T) {
	ctx := context.Background()
	_, releaseRepo, host := obeyFixture(t, "0.2.0")

	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}

	out, errOut, err := runInstaller(t, "update", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update obey: %v\n%s", err, errOut)
	}
	var current obeyUpdateData
	dataOf(t, out, &current)
	if current.Action != "current" || current.Version != "0.2.0" {
		t.Fatalf("expected current at 0.2.0, got %+v", current)
	}
	if current.Package != app.ObeyPackageID {
		t.Fatalf("package = %q, want %q", current.Package, app.ObeyPackageID)
	}

	host.publish("0.2.1", obeyTarball(t, "0.2.1"))
	git(t, releaseRepo, "tag", "v0.2.1")

	out, errOut, err = runInstaller(t, "update", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("update obey to 0.2.1: %v\n%s", err, errOut)
	}
	var upgraded obeyUpdateData
	dataOf(t, out, &upgraded)
	if upgraded.Action != "upgraded" || upgraded.From != "0.2.0" || upgraded.Version != "0.2.1" {
		t.Fatalf("expected upgraded 0.2.0 -> 0.2.1, got %+v", upgraded)
	}
	if upgraded.SelfReplaced {
		t.Fatalf("self_replaced must stay false for obey: %+v", upgraded)
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	for name, want := range map[string]string{
		"obey": obeyBodyPrefix + "0.2.1\n",
		"ob":   obBodyPrefix + "0.2.1\n",
	} {
		got, readErr := os.ReadFile(filepath.Join(binDir, name))
		if readErr != nil || string(got) != want {
			t.Fatalf("%s after update = %q (err %v), want %q", name, got, readErr, want)
		}
	}
}

func TestUpdateObey_AbsentAndUnmanaged(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("FESTIVAL_HOME", "")
	t.Setenv("HOME", t.TempDir())
	// An empty PATH entry keeps a host-installed obey out of ResolveTool, so
	// "absent" is measured against the fixture and not the developer machine.
	t.Setenv("PATH", t.TempDir())

	out, errOut, err := runInstaller(t, "update", "obey", "--json")
	if err != nil {
		t.Fatalf("update obey with nothing installed: %v\n%s", err, errOut)
	}
	var absent obeyUpdateData
	dataOf(t, out, &absent)
	if absent.Action != "absent" {
		t.Fatalf("expected absent, got %+v", absent)
	}
	if !strings.Contains(out, "festival install obey") {
		t.Fatalf("expected the absent warning to name `festival install obey`:\n%s", out)
	}
	if !strings.Contains(errOut, "festival install obey") {
		t.Fatalf("expected the absent warning on stderr too, got %q", errOut)
	}

	external := t.TempDir()
	writeFile(t, filepath.Join(external, "obey"), "#!/bin/sh\necho obey 9.9.9\n")
	if err := os.Chmod(filepath.Join(external, "obey"), 0o755); err != nil {
		t.Fatalf("chmod external obey: %v", err)
	}
	t.Setenv("PATH", external+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, errOut, err = runInstaller(t, "update", "obey", "--json")
	if err != nil {
		t.Fatalf("update obey with an external obey: %v\n%s", err, errOut)
	}
	var unmanaged obeyUpdateData
	dataOf(t, out, &unmanaged)
	if unmanaged.Action != "unmanaged" {
		t.Fatalf("expected unmanaged, got %+v", unmanaged)
	}
	if !strings.Contains(out, filepath.Join(external, "obey")) {
		t.Fatalf("expected the unmanaged warning to name the external path:\n%s", out)
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	for _, name := range []string{"obey", "ob"} {
		if _, statErr := os.Stat(filepath.Join(binDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("an unmanaged obey must not be replaced: %s exists (err %v)", name, statErr)
		}
	}
}

func TestUninstallObey_RemovesBothBinaries(t *testing.T) {
	ctx := context.Background()
	obeyFixture(t, "0.2.0")

	if _, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json"); err != nil {
		t.Fatalf("install obey: %v\n%s", err, errOut)
	}

	out, errOut, err := runInstaller(t, "uninstall", "obey", "--json")
	if err != nil {
		t.Fatalf("uninstall obey: %v\n%s", err, errOut)
	}
	var res struct {
		Package string   `json:"package"`
		Removed []string `json:"removed"`
	}
	dataOf(t, out, &res)
	if res.Package != app.ObeyPackageID {
		t.Fatalf("package = %q, want %q", res.Package, app.ObeyPackageID)
	}
	if len(res.Removed) != 2 {
		t.Fatalf("expected both binaries removed, got %v", res.Removed)
	}

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	for _, name := range []string{"obey", "ob"} {
		p := filepath.Join(binDir, name)
		if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
			t.Fatalf("%s should be removed, stat err=%v", p, statErr)
		}
		if !slices.Contains(res.Removed, p) {
			t.Fatalf("removed list is missing %s: %v", p, res.Removed)
		}
	}

	listOut, _, err := runInstaller(t, "list", "--json")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Contains(listOut, app.ObeyPackageID) {
		t.Fatalf("list still reports the obey package after uninstall:\n%s", listOut)
	}
}

// fixtureBareBinaryMarketplace publishes obey as a single file rather than an
// archive, which is the shape the product install path cannot place.
func fixtureBareBinaryMarketplace(t *testing.T, repoPath, baseURL string) string {
	t.Helper()
	root := fmt.Sprintf(`{
  "id": "obedience-corp/official",
  "name": "Official",
  "schema_version": "1",
  "packages": [
    {
      "id": "obedience-corp/obey",
      "display_name": "Obey Daemon",
      "class": "product",
      "host_runtimes": ["obey"],
      "channels": ["stable"],
      "release_source": {
        "type": "git",
        "repo": %q,
        "asset_url": "%s/dl/obey-{version}-{os}-{arch}",
        "checksums_url": "%s/dl/checksums.txt",
        "binaries": ["obey"]
      }
    }
  ]
}
`, repoPath, baseURL, baseURL)

	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), root)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	return dir
}

// obeyBareAssetName is obeyAssetName without the archive suffix.
func obeyBareAssetName(version string) string {
	return strings.TrimSuffix(obeyAssetName(version), ".tar.gz")
}

// A product that publishes a bare binary is refused for that, and refused
// before the artifact is pulled and hashed: everything the refusal depends on
// is in the release_source template.
func TestInstallObey_BareBinaryRefusedBeforeTheDownload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	t.Setenv("FESTIVAL_HOME", "")
	t.Setenv("HOME", t.TempDir())

	releaseRepo := gitReleaseRepo(t, "v0.2.0")
	host := newObeyReleaseHost(t)
	host.publishNamed(obeyBareAssetName("0.2.0"), []byte(obeyBodyPrefix+"0.2.0\n"))

	mkt := fixtureBareBinaryMarketplace(t, releaseRepo, host.url())
	if _, errOut, err := runInstaller(t, "marketplace", "add", mkt, "--name", "official-obey", "--allow-unverified"); err != nil {
		t.Fatalf("marketplace add: %v\n%s", err, errOut)
	}

	_, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json")
	if err == nil {
		t.Fatal("expected a bare-binary obey release to be refused")
	}
	if !hasErrorCode(err, "E_PRODUCT_NOT_ARCHIVE") && !strings.Contains(errOut, "E_PRODUCT_NOT_ARCHIVE") {
		t.Fatalf("expected E_PRODUCT_NOT_ARCHIVE, got err=%v errOut=%s", err, errOut)
	}
	if strings.Contains(err.Error(), "more than one binary") {
		t.Fatalf("the refusal must not blame the binary count: %v", err)
	}
	if got := host.assetDownloads(); got != 0 {
		t.Fatalf("the artifact was fetched %d time(s) before the refusal, want 0", got)
	}
}

// fixtureBrokenExtraMarketplace registers a second, unrelated marketplace and
// corrupts its clone, which is the state a third-party source lands in after a
// bad publish.
func fixtureBrokenExtraMarketplace(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	root := `{
  "id": "acme/extra",
  "name": "Extra",
  "schema_version": "1",
  "packages": []
}
`
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), root)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	if _, errOut, err := runInstaller(t, "marketplace", "add", dir, "--name", name, "--allow-unverified"); err != nil {
		t.Fatalf("marketplace add %s: %v\n%s", name, err, errOut)
	}
	clone, err := source.CloneDir(ctx, name)
	if err != nil {
		t.Fatalf("CloneDir %s: %v", name, err)
	}
	writeFile(t, filepath.Join(clone, "obey-marketplace.json"), "{ this is not a marketplace")
}

// obey resolves through the source that declares it, so a third-party
// marketplace that fails to load is that marketplace's problem. Before this,
// any registered source that could not parse decided whether obey was
// installable, while installing the suite from its own source still worked.
func TestInstallObey_UnrelatedBrokenMarketplaceDoesNotBlockIt(t *testing.T) {
	ctx := context.Background()
	home, _, _ := obeyFixture(t, "0.2.0")
	fixtureBrokenExtraMarketplace(t, ctx, "extra")

	// The corruption has to be load-blocking, or this test proves nothing:
	// browse still walks every source and must fail on it.
	if _, _, err := runInstaller(t, "browse", "--allow-unverified", "--json"); err == nil {
		t.Fatal("expected browse to fail on the broken marketplace")
	}

	out, errOut, err := runInstaller(t, "install", "obey", "--allow-unverified", "--json")
	if err != nil {
		t.Fatalf("a broken unrelated marketplace must not block obey: %v\n%s", err, errOut)
	}
	var res obeyInstallData
	dataOf(t, out, &res)
	if len(res.Files) != 2 {
		t.Fatalf("expected both obey binaries, got %v", res.Files)
	}
	if strings.Contains(errOut, "extra") {
		t.Fatalf("the install must not warn about a marketplace it never read: %q", errOut)
	}
	if _, err := receipts.Get(ctx, mustDB(t, ctx, home), app.ObeyPackageID); err != nil {
		t.Fatalf("obey receipt: %v", err)
	}
}
