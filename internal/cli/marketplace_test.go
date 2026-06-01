package cli_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/obey-installer/internal/cli"
	"github.com/Obedience-Corp/obey-installer/internal/source"
)

const validManifest = `{
  "id": "obedience-corp/official",
  "name": "Official",
  "schema_version": "1",
  "packages": [
    {
      "id": "obedience-corp/fest",
      "display_name": "Fest",
      "class": "tool",
      "manifest_path": "packages/fest/obey-package.json"
    }
  ]
}
`

func gitEnv() []string {
	return append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func fixtureMarketplace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	writeFile(t, filepath.Join(dir, "obey-marketplace.json"), validManifest)
	writeFile(t, filepath.Join(dir, "packages", "fest", "obey-package.json"), "{}\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "init")
	return dir
}

func listContainsPackageCount(out, name string, count int) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, name) {
			fields := strings.Fields(line)
			return len(fields) > 0 && fields[len(fields)-1] == strconv.Itoa(count)
		}
	}
	return false
}

func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "obey-installer", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(cli.NewMarketplaceCommand())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestMarketplaceE2E_AddListRefreshRemove(t *testing.T) {
	fixture := fixtureMarketplace(t)
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()

	if out, err := runCmd(t, "marketplace", "add", fixture, "--name", "official"); err != nil {
		t.Fatalf("add: %v\n%s", err, out)
	}

	clone := filepath.Join(home, "marketplaces", "official")
	if _, err := os.Stat(clone); err != nil {
		t.Fatalf("clone missing after add: %v", err)
	}

	listOut, err := runCmd(t, "marketplace", "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, listOut)
	}
	if !strings.Contains(listOut, "official") {
		t.Fatalf("list missing source:\n%s", listOut)
	}
	if !listContainsPackageCount(listOut, "official", 1) {
		t.Fatalf("list missing package count for official:\n%s", listOut)
	}

	writeFile(t, filepath.Join(fixture, "CHANGELOG.md"), "v2\n")
	git(t, fixture, "add", ".")
	git(t, fixture, "commit", "-m", "second")

	refreshOut, err := runCmd(t, "marketplace", "refresh", "official", "--json")
	if err != nil {
		t.Fatalf("refresh: %v\n%s", err, refreshOut)
	}
	if !strings.Contains(refreshOut, "\"changed\": true") {
		t.Fatalf("refresh did not report change:\n%s", refreshOut)
	}

	if out, err := runCmd(t, "marketplace", "remove", "official"); err != nil {
		t.Fatalf("remove: %v\n%s", err, out)
	}
	if _, err := os.Stat(clone); !os.IsNotExist(err) {
		t.Fatalf("clone still present after remove: %v", err)
	}
	sources, err := source.ListMarketplaces(ctx)
	if err != nil {
		t.Fatalf("ListMarketplaces: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("registry not empty after remove: %+v", sources)
	}
}

func TestMarketplaceE2E_AddNonMarketplaceRollsBack(t *testing.T) {
	bare := t.TempDir()
	git(t, bare, "init", "-b", "main")
	writeFile(t, filepath.Join(bare, "README.md"), "not a marketplace\n")
	git(t, bare, "add", ".")
	git(t, bare, "commit", "-m", "init")

	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()

	if _, err := runCmd(t, "marketplace", "add", bare, "--name", "bogus"); err == nil {
		t.Fatal("expected add to fail for repo without obey-marketplace.json")
	}

	if _, err := os.Stat(filepath.Join(home, "marketplaces", "bogus")); !os.IsNotExist(err) {
		t.Fatalf("clone should be rolled back: %v", err)
	}
	sources, err := source.ListMarketplaces(ctx)
	if err != nil {
		t.Fatalf("ListMarketplaces: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("registry should be empty after rollback: %+v", sources)
	}
}
