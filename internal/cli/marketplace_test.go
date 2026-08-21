package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Obedience-Corp/festival-installer/internal/cli"
	"github.com/Obedience-Corp/festival-installer/internal/jsonout"
	"github.com/Obedience-Corp/festival-installer/internal/source"
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
	root := &cobra.Command{Use: "festival", SilenceErrors: true, SilenceUsage: true}
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
	sources, err := source.ListMarketplaces(ctx, source.DefaultVerifyOptions(nil, false))
	if err != nil {
		t.Fatalf("ListMarketplaces: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("registry not empty after remove: %+v", sources)
	}
}

type marketplaceEnvelope struct {
	OK            bool            `json:"ok"`
	Action        string          `json:"action"`
	SchemaVersion string          `json:"schema_version"`
	Warnings      []string        `json:"warnings"`
	Data          json.RawMessage `json:"data"`
}

func decodeMarketplaceEnvelope(t *testing.T, out string) marketplaceEnvelope {
	t.Helper()
	var env marketplaceEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	if !env.OK {
		t.Fatalf("envelope not ok: %s", out)
	}
	if env.SchemaVersion != jsonout.SchemaVersion {
		t.Fatalf("schema_version = %q, want %q", env.SchemaVersion, jsonout.SchemaVersion)
	}
	return env
}

const seedFriendlyWarning = "couldn't reach the official marketplace; showing local sources only"

func TestMarketplaceListRefreshJSONEnvelope(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		action string
	}{
		{"list", []string{"marketplace", "list", "--json"}, "marketplace list"},
		{"refresh", []string{"marketplace", "refresh", "--json"}, "marketplace refresh"},
	}
	for _, tc := range cases {
		// Sequence 05 verifies every read of a registered source. "official"
		// here is an ordinary added name (not state.OfficialSeedKey), so it
		// gets PolicyWarnAllow: unsigned still works, but now legitimately
		// carries the loud "UNVERIFIED content" warning on stderr. That is
		// this festival's intended behavior, distinct from the seed warning
		// this subtest's name refers to (the envelope "warnings" field, which
		// must still stay empty since there is no seed failure here).
		t.Run(tc.name+"/seeded home has empty envelope warnings", func(t *testing.T) {
			t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
			fixture := fixtureMarketplace(t)
			if out, errOut, err := runInstaller(t, "marketplace", "add", fixture, "--name", "official"); err != nil {
				t.Fatalf("add: %v\n%s%s", err, out, errOut)
			}

			out, errOut, err := runInstaller(t, tc.args...)
			if err != nil {
				t.Fatalf("%v: %v\n%s", tc.args, err, errOut)
			}
			if !strings.Contains(errOut, "UNVERIFIED") {
				t.Fatalf("expected the unverified-content warning on stderr, got %q", errOut)
			}
			if strings.Contains(errOut, seedFriendlyWarning) {
				t.Fatalf("expected no seed warning on stderr, got %q", errOut)
			}
			env := decodeMarketplaceEnvelope(t, out)
			if env.Action != tc.action {
				t.Fatalf("action = %q, want %q", env.Action, tc.action)
			}
			if env.Warnings == nil || len(env.Warnings) != 0 {
				t.Fatalf("expected warnings to serialize as [], got %v in %s", env.Warnings, out)
			}
			var views []map[string]any
			if err := json.Unmarshal(env.Data, &views); err != nil {
				t.Fatalf("data is not an array of views: %v\n%s", err, out)
			}
			if len(views) != 1 || views[0]["name"] != "official" {
				t.Fatalf("expected the registered source in data, got %s", env.Data)
			}
		})
		// GIT_ALLOW_PROTOCOL=file makes the https seed clone fail deterministically
		// with raw git output, without touching the network.
		t.Run(tc.name+"/seed failure lands in warnings without git trace", func(t *testing.T) {
			t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
			t.Setenv("GIT_ALLOW_PROTOCOL", "file")

			out, errOut, err := runInstaller(t, tc.args...)
			if err != nil {
				t.Fatalf("%v: %v\n%s", tc.args, err, errOut)
			}
			if errOut != "" {
				t.Fatalf("expected the warning in the envelope only, got stderr %q", errOut)
			}
			env := decodeMarketplaceEnvelope(t, out)
			if len(env.Warnings) != 1 || env.Warnings[0] != seedFriendlyWarning {
				t.Fatalf("warnings = %v, want [%q]", env.Warnings, seedFriendlyWarning)
			}
			for _, leak := range []string{"clone", "fatal", "github.com"} {
				if strings.Contains(out, leak) {
					t.Fatalf("raw git detail %q leaked into the envelope: %s", leak, out)
				}
			}
			if !strings.Contains(out, "\"data\": []") {
				t.Fatalf("expected empty views to serialize as [], got: %s", out)
			}
		})
		t.Run(tc.name+"/table mode warns on stderr without git trace", func(t *testing.T) {
			t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
			t.Setenv("GIT_ALLOW_PROTOCOL", "file")

			tableArgs := tc.args[:len(tc.args)-1]
			out, errOut, err := runInstaller(t, tableArgs...)
			if err != nil {
				t.Fatalf("%v: %v\n%s", tableArgs, err, errOut)
			}
			if !strings.Contains(errOut, "warning: "+seedFriendlyWarning) {
				t.Fatalf("expected friendly warning on stderr, got %q", errOut)
			}
			for _, leak := range []string{"clone", "fatal", "github.com"} {
				if strings.Contains(out+errOut, leak) {
					t.Fatalf("raw git detail %q leaked: %s%s", leak, out, errOut)
				}
			}
		})
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
	sources, err := source.ListMarketplaces(ctx, source.DefaultVerifyOptions(nil, false))
	if err != nil {
		t.Fatalf("ListMarketplaces: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("registry should be empty after rollback: %+v", sources)
	}
}
