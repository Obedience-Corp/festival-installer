package release_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeChecksumsCmd is the checksum step in .github/workflows/release.yml.
// Hashing from inside dist/ emits basenames so `shasum -a 256 -c checksums.txt`
// works next to assets downloaded by `gh release download`.
const writeChecksumsCmd = "(cd dist && sha256sum festival-* > checksums.txt)"

func TestReleaseWorkflowWritesChecksumsFromInsideDist(t *testing.T) {
	body := readReleaseWorkflow(t)
	if strings.Contains(body, "sha256sum dist/festival-*") {
		t.Fatal("release.yml hashes dist/festival-* from the repo root; checksums.txt then names dist/festival-* and shasum -c fails next to downloaded assets")
	}
	if !strings.Contains(body, writeChecksumsCmd) {
		t.Fatalf("release.yml must write checksums.txt from inside dist; want %q", writeChecksumsCmd)
	}
}

func TestChecksumsTxtBesideDownloadedAssets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release.yml runs on ubuntu-latest")
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Skip("sha256sum not on PATH")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}

	names := []string{
		"festival-darwin-amd64",
		"festival-darwin-arm64",
		"festival-linux-amd64",
		"festival-linux-arm64",
	}

	t.Run("basename paths verify", func(t *testing.T) {
		download := writeChecksumsAndDownload(t, names, writeChecksumsCmd)
		assertNoDistPrefix(t, filepath.Join(download, "checksums.txt"))
		if out, err := checkSHA256(t, download); err != nil {
			t.Fatalf("shasum -c next to downloaded assets: %v\n%s", err, out)
		}
	})

	t.Run("dist prefixes fail", func(t *testing.T) {
		download := writeChecksumsAndDownload(t, names, "sha256sum dist/festival-* > dist/checksums.txt")
		if out, err := checkSHA256(t, download); err == nil {
			t.Fatalf("expected shasum -c to fail on dist/ prefixes, got:\n%s", out)
		}
	})
}

func readReleaseWorkflow(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", ".github", "workflows", "release.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeChecksumsAndDownload(t *testing.T, names []string, checksumCmd string) string {
	t.Helper()
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	if err := os.Mkdir(dist, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dist, name), []byte("payload-"+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", "-c", checksumCmd)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("write checksums (%s): %v\n%s", checksumCmd, err, out)
	}

	download := t.TempDir()
	entries, err := os.ReadDir(dist)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dist, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(download, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return download
}

func assertNoDistPrefix(t *testing.T, checksumsPath string) {
	t.Helper()
	b, err := os.ReadFile(checksumsPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.Contains(text, "dist/") {
		t.Fatalf("checksums.txt still has dist/ prefixes:\n%s", text)
	}
	if !strings.Contains(text, "festival-darwin-amd64") {
		t.Fatalf("checksums.txt missing artifact names:\n%s", text)
	}
}

func checkSHA256(t *testing.T, dir string) (string, error) {
	t.Helper()
	var cmd *exec.Cmd
	if _, err := exec.LookPath("shasum"); err == nil {
		cmd = exec.Command("shasum", "-a", "256", "-c", "checksums.txt")
	} else {
		cmd = exec.Command("sha256sum", "-c", "checksums.txt")
	}
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
