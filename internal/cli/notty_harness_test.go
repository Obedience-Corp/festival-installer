//go:build notty

package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// headlessBudget bounds one command. A go test -timeout failure names the whole
// package; this deadline names the command that hung.
const headlessBudget = 120 * time.Second

// festivalBinary is the real binary every case in this lane drives. The
// in-process cobra tree the other tests assemble cannot prove what this lane
// claims: its stdout is a bytes.Buffer, so cmdStdioIsTTY short-circuits on a
// failed type assertion rather than on a missing terminal, and
// tuiLaunchDecision is never reached at all.
var festivalBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "notty-festival-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "notty lane: temp dir: %v\n", err)
		os.Exit(1)
	}
	code := func() int {
		defer func() { _ = os.RemoveAll(dir) }()
		bin, buildErr := buildFestival(dir)
		if buildErr != nil {
			fmt.Fprintf(os.Stderr, "notty lane: %v\n", buildErr)
			return 1
		}
		festivalBinary = bin
		return m.Run()
	}()
	os.Exit(code)
}

// buildFestival compiles the real binary once per lane run.
func buildFestival(dir string) (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "festival")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/festival")
	cmd.Dir = root
	if out, buildErr := cmd.CombinedOutput(); buildErr != nil {
		return "", errors.New("build festival: " + buildErr.Error() + "\n" + string(out))
	}
	return bin, nil
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found above the test working directory")
		}
		dir = parent
	}
}

type headlessRun struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// runHeadless runs the real binary with stdin on /dev/null, stdout and stderr
// on pipes, and its own session so it has no controlling terminal. This is the
// shape festival-app spawns the installer in from a Dock or Finder launch.
func runHeadless(ctx context.Context, t *testing.T, bin string, env []string, args ...string) headlessRun {
	t.Helper()
	runCtx, cancel := context.WithTimeout(ctx, headlessBudget)
	defer cancel()

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(runCtx, bin, args...)
	cmd.Stdin = devNull
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	runErr := cmd.Run()
	run := headlessRun{Stdout: stdout.String(), Stderr: stderr.String()}
	line := "festival " + strings.Join(args, " ")
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr):
		run.ExitCode = exitErr.ExitCode()
	default:
		t.Fatalf("run %s: %v\nstdout:\n%s\nstderr:\n%s", line, runErr, run.Stdout, run.Stderr)
	}
	if runCtx.Err() != nil {
		t.Fatalf("%s did not finish within %s", line, headlessBudget)
	}
	t.Logf("%s -> exit %d", line, run.ExitCode)
	return run
}

// assertSingleJSONObject decodes stdout and asserts nothing follows the first
// value. A stray fmt.Println on stdout is the failure mode this catches, and it
// is the one that breaks festival-app's parser.
func assertSingleJSONObject(t *testing.T, out string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(out))
	var first map[string]any
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("stdout is not a JSON object: %v\n%s", err, out)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout carried more than one value (err=%v):\n%s", err, out)
	}
	return first
}

// headlessEnv is a deliberately minimal environment, closer to a Dock launch
// than to a shell. It keeps HOME and PATH, points FESTIVAL_HOME at a temp dir,
// and carries nothing else. Directories in front are searched before the
// minimal tool directory.
func headlessEnv(t *testing.T, home, festivalHome string, front ...string) []string {
	t.Helper()
	dirs := make([]string, 0, len(front)+1)
	dirs = append(dirs, front...)
	dirs = append(dirs, minimalPATH(t))
	return []string{
		"HOME=" + home,
		"PATH=" + strings.Join(dirs, string(os.PathListSeparator)),
		"FESTIVAL_HOME=" + festivalHome,
	}
}

// minimalPATH holds only the host tools the installer shells out to: git for
// the marketplace clone and the release tag walk, and sh, tar, and gzip for the
// fixture binaries. Nothing else, so no host camp, fest, or festival can leak
// into a headless run.
func minimalPATH(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"git", "sh", "tar", "gzip"} {
		p, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("host tool %s not found: %v", name, err)
		}
		if err := os.Symlink(p, filepath.Join(dir, name)); err != nil {
			t.Fatalf("symlink %s: %v", name, err)
		}
	}
	return dir
}

// headlessFixture is one prepared machine: an isolated user home, an isolated
// FESTIVAL_HOME that does not exist yet, a fixture marketplace carrying both
// the suite and obey, and the minimal environment.
type headlessFixture struct {
	Marketplace  string
	FestivalHome string
	BinDir       string
	Env          []string
	// PackageBin is the fake package-manager suite bin dir, empty unless the
	// fixture staged a package origin.
	PackageBin string
	// BrewLog is the file the recording fake brew appends to, empty unless the
	// fixture staged a package origin.
	BrewLog string
}

func newHeadlessFixture(t *testing.T) headlessFixture {
	t.Helper()
	return buildHeadlessFixture(t, false)
}

// newPackageOriginFixture adds a fake Homebrew suite prefix and a recording
// fake brew to the plain fixture. HOMEBREW_PREFIX plus the share/festival/shell
// helper is what classifyPath keys on to report package origin with the
// homebrew flavor, which is the only flavor whose upgrade line is a runnable
// command rather than a URL.
func newPackageOriginFixture(t *testing.T) headlessFixture {
	t.Helper()
	return buildHeadlessFixture(t, true)
}

func buildHeadlessFixture(t *testing.T, packageOrigin bool) headlessFixture {
	t.Helper()
	userHome := t.TempDir()
	festivalHome := filepath.Join(t.TempDir(), "installer")

	suite := buildSuiteTarGz(t, map[string]string{
		"camp":     suiteVersionScript("camp", "v0.2.11"),
		"fest":     suiteVersionScript("fest", "v0.4.5"),
		"festival": festivalVersionScript("v0.2.10"),
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(suite)
	}))
	t.Cleanup(srv.Close)

	releaseRepo := gitReleaseRepo(t, "v0.2.0")
	host := newObeyReleaseHost(t)
	host.publish("0.2.0", obeyTarball(t, "0.2.0"))

	fx := headlessFixture{
		Marketplace:  fixtureCombinedMarketplace(t, srv.URL+"/festival.tar.gz", sha256Hex(suite), releaseRepo, host.url()),
		FestivalHome: festivalHome,
		BinDir:       filepath.Join(festivalHome, "bin"),
	}
	if !packageOrigin {
		fx.Env = headlessEnv(t, userHome, festivalHome)
		return fx
	}

	// A deliberately old suite version, so channel-latest is always newer and
	// PackageUpgradeAvailable is true. That is what carries the update path past
	// its availability check and onto the cmdStdioIsTTY gate this lane measures.
	fx.PackageBin = packageOriginPrefix(t, "0.1.0")
	brewDir, brewLog := recordingBrew(t)
	fx.BrewLog = brewLog
	fx.Env = append(
		headlessEnv(t, userHome, festivalHome, brewDir, fx.PackageBin),
		"HOMEBREW_PREFIX="+filepath.Dir(fx.PackageBin),
	)
	return fx
}

func (f headlessFixture) run(ctx context.Context, t *testing.T, args ...string) headlessRun {
	t.Helper()
	return runHeadless(ctx, t, festivalBinary, f.Env, args...)
}

func (f headlessFixture) addMarketplace(ctx context.Context, t *testing.T) {
	t.Helper()
	run := f.run(ctx, t, "marketplace", "add", f.Marketplace, "--name", "official-obey", "--allow-unverified")
	if run.ExitCode != 0 {
		t.Fatalf("marketplace add exit = %d\nstdout:\n%s\nstderr:\n%s", run.ExitCode, run.Stdout, run.Stderr)
	}
}

// assertNoPackageManagerRan fails when the recording fake brew was invoked. It
// is the measurement behind maybeRunPackageUpgrade's non-TTY early return.
func (f headlessFixture) assertNoPackageManagerRan(t *testing.T) {
	t.Helper()
	body, err := os.ReadFile(f.BrewLog)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("read brew log: %v", err)
	}
	if len(bytes.TrimSpace(body)) != 0 {
		t.Fatalf("a package-manager upgrade ran headlessly; brew log:\n%s", body)
	}
}

// recordingBrew stages a fake brew that appends its argv to a log and exits 0,
// so LookPath finds it and any invocation is recorded.
func recordingBrew(t *testing.T) (dir, logPath string) {
	t.Helper()
	dir = t.TempDir()
	logPath = filepath.Join(t.TempDir(), "brew-invocations.log")
	script := "#!/bin/sh\necho \"$@\" >> " + logPath + "\nexit 0\n"
	path := filepath.Join(dir, "brew")
	writeFile(t, path, script)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("chmod fake brew: %v", err)
	}
	return dir, logPath
}

// fixtureCombinedMarketplace carries both products in one document, because the
// lane drives a suite install and an obey install against the same registered
// source, exactly as a real machine does.
func fixtureCombinedMarketplace(t *testing.T, artifactURL, artifactSha, releaseRepo, releaseBaseURL string) string {
	t.Helper()
	root := fmt.Sprintf(`{
  "id": "obedience-corp/official",
  "name": "Official",
  "schema_version": "1",
  "packages": [
    {"id": "obedience-corp/festival", "display_name": "Festival", "class": "product", "manifest_path": "packages/obedience-corp/festival/obey-package.json"},
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
`, releaseRepo, releaseBaseURL, releaseBaseURL)

	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), root)
	writeFile(t, filepath.Join(dir, "packages", "obedience-corp", "festival", "obey-package.json"),
		productManifest(artifactURL, artifactSha))
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	return dir
}
