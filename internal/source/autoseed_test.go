package source

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/state"
)

const seedTestManifest = `{
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

func seedTestCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func seedGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func seedFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	seedGit(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, manifestFilename), []byte(seedTestManifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "packages", "fest"), 0o755); err != nil {
		t.Fatalf("mkdir packages: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "packages", "fest", "obey-package.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write package: %v", err)
	}
	seedGit(t, dir, "add", ".")
	seedGit(t, dir, "commit", "-m", "init")
	return dir
}

func seedHome(t *testing.T) {
	t.Helper()
	t.Setenv("OBEY_INSTALLER_HOME", t.TempDir())
}

func sourceNames(t *testing.T, ctx context.Context) []string {
	t.Helper()
	views, err := ListMarketplaces(ctx)
	if err != nil {
		t.Fatalf("ListMarketplaces: %v", err)
	}
	names := make([]string, 0, len(views))
	for _, v := range views {
		names = append(names, v.Name)
	}
	return names
}

func markerRecorded(t *testing.T, ctx context.Context) bool {
	t.Helper()
	home, err := state.Home(ctx)
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer func() { _ = db.Close(ctx) }()
	ok, err := state.SeedMarkerExists(ctx, db.Raw(), state.OfficialSeedKey)
	if err != nil {
		t.Fatalf("SeedMarkerExists: %v", err)
	}
	return ok
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	os.Stderr = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

func TestEnsureOfficialSeed_FirstSeedIsSilent(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	fixture := seedFixtureRepo(t)

	prev := officialMarketplaceURL
	officialMarketplaceURL = fixture
	t.Cleanup(func() { officialMarketplaceURL = prev })

	stderr := captureStderr(t, func() {
		if err := EnsureOfficialSeed(ctx); err != nil {
			t.Fatalf("EnsureOfficialSeed: %v", err)
		}
	})
	if stderr != "" {
		t.Fatalf("first successful seed must not write to stderr, got %q", stderr)
	}

	names := sourceNames(t, ctx)
	if len(names) != 1 || names[0] != state.OfficialSeedKey {
		t.Fatalf("expected only %q after seed, got %v", state.OfficialSeedKey, names)
	}
	if !markerRecorded(t, ctx) {
		t.Fatal("seed marker should be recorded after a successful seed")
	}
}

func TestAutoseed_FreshStateSeedsOnce(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	fixture := seedFixtureRepo(t)

	added, err := seedFromURL(ctx, fixture)
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if !added {
		t.Fatal("first seed should have added the official source")
	}

	names := sourceNames(t, ctx)
	if len(names) != 1 || names[0] != state.OfficialSeedKey {
		t.Fatalf("expected only %q after seed, got %v", state.OfficialSeedKey, names)
	}
	if !markerRecorded(t, ctx) {
		t.Fatal("seed marker should be recorded after a successful seed")
	}

	added, err = seedFromURL(ctx, fixture)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if added {
		t.Fatal("second seed must be a no-op")
	}
	if names := sourceNames(t, ctx); len(names) != 1 {
		t.Fatalf("second seed changed sources: %v", names)
	}
}

func TestAutoseed_RespectsUserRemoval(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	fixture := seedFixtureRepo(t)

	if _, err := seedFromURL(ctx, fixture); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := RemoveMarketplace(ctx, state.OfficialSeedKey); err != nil {
		t.Fatalf("remove: %v", err)
	}

	added, err := seedFromURL(ctx, fixture)
	if err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if added {
		t.Fatal("seed must not re-add a source the user removed")
	}
	if names := sourceNames(t, ctx); len(names) != 0 {
		t.Fatalf("expected no sources after user removal, got %v", names)
	}
}

func TestAutoseed_ExistingSourcesUntouched(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)
	userFixture := seedFixtureRepo(t)
	officialFixture := seedFixtureRepo(t)

	if _, err := AddMarketplace(ctx, userFixture, "acme"); err != nil {
		t.Fatalf("add user source: %v", err)
	}

	added, err := seedFromURL(ctx, officialFixture)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if added {
		t.Fatal("seed must not add the official source when the user already has sources")
	}

	names := sourceNames(t, ctx)
	if len(names) != 1 || names[0] != "acme" {
		t.Fatalf("expected only the user source, got %v", names)
	}
	if !markerRecorded(t, ctx) {
		t.Fatal("seed marker should be recorded even when seeding is skipped for existing sources")
	}

	if added, err := seedFromURL(ctx, officialFixture); err != nil || added {
		t.Fatalf("subsequent seed should be a no-op (added=%v err=%v)", added, err)
	}
}

func TestAutoseed_OfflineFailureDoesNotRecordMarker(t *testing.T) {
	seedHome(t)
	ctx := seedTestCtx(t)

	added, err := seedFromURL(ctx, "file:///nonexistent/marketplace.git")
	if err == nil {
		t.Fatal("expected seed to fail for an unreachable URL")
	}
	if added {
		t.Fatal("failed seed must not report an add")
	}
	if markerRecorded(t, ctx) {
		t.Fatal("a failed seed must not record the marker (it must stay retryable)")
	}

	fixture := seedFixtureRepo(t)
	added, err = seedFromURL(ctx, fixture)
	if err != nil {
		t.Fatalf("retry seed: %v", err)
	}
	if !added {
		t.Fatal("retry after an offline failure should seed")
	}
}
