//go:build (container_fs && linux) || (darwin && app_native)

package installer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

func TestAppReconcileLeavesInstallIntentAlone(t *testing.T) {
	requireAppIsolation(t)
	ctx := context.Background()
	s := appScenario{metadata.ArtifactKindAppBundle, false}
	home, db := newAppHome(t)
	parent := t.TempDir()
	tx, err := BeginApp(ctx, db, home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Prepare(ctx, AppPlacement{PackageID: appTestPkg, Version: "0.1.8", Format: s.format, ArtifactSHA256: appTestNewSHA, Dest: s.dest(parent)}); err != nil {
		t.Fatal(err)
	}
	abandon(tx)
	sentinel := []byte(`{"id":"sentinel","placed":[]}`)
	if err := os.WriteFile(filepath.Join(home, JournalName), sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileApp(ctx, home, db); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, JournalName))
	if err != nil || string(got) != string(sentinel) {
		t.Fatalf("install-intent.json changed: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(home, IntentBackupsDir)); !os.IsNotExist(err) {
		t.Fatalf("intent-backups created: %v", err)
	}
	assertNoAppJournal(t, home)
	assertNoSiblings(t, parent)
}

func TestAppReconcileRejectsForgedJournal(t *testing.T) {
	requireAppIsolation(t)
	ctx := context.Background()
	hex32 := strings.Repeat("a", 32)
	id := "0123456789abcdef"
	cases := map[string]func(parent string) AppIntent{
		"staged is a victim directory": func(parent string) AppIntent {
			dest := filepath.Join(parent, "App")
			return AppIntent{ID: id, Dest: dest, Staged: filepath.Join(parent, "victim"), Payload: filepath.Join(parent, "victim", "App"), Phase: AppPhaseStaging}
		},
		"staged names another base": func(parent string) AppIntent {
			dest := filepath.Join(parent, "App")
			staged := filepath.Join(parent, ".Other.festival-new-"+id)
			return AppIntent{ID: id, Dest: dest, Staged: staged, Payload: filepath.Join(staged, "App"), Phase: AppPhaseStaging}
		},
		"relative dest": func(parent string) AppIntent {
			return AppIntent{ID: id, Dest: "App", Staged: ".App.festival-new-" + id, Payload: ".App.festival-new-" + id + "/App", Phase: AppPhaseStaging}
		},
		"32-hex id in a placement phase": func(parent string) AppIntent {
			dest := filepath.Join(parent, "App")
			return AppIntent{ID: hex32, Dest: dest, Staged: stagedSiblingPath(dest, hex32), Payload: filepath.Join(stagedSiblingPath(dest, hex32), "App"), Phase: AppPhaseStaging}
		},
		"32-hex id in an external entry": func(parent string) AppIntent {
			return AppIntent{ID: hex32, Dest: "dpkg:festival-app", Phase: AppPhaseExternal}
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			home, _ := newAppHome(t)
			parent := t.TempDir()
			victim := filepath.Join(parent, "victim")
			if err := os.MkdirAll(victim, 0o755); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(parent, ".Other.festival-new-"+id)
			if err := os.MkdirAll(other, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := writeAppIntent(home, build(parent)); err != nil {
				t.Fatal(err)
			}
			err := ReconcileApp(ctx, home, nil)
			if errpkg.Code(err) != "E_INSTALL_RECONCILE" {
				t.Fatalf("error = %v, want E_INSTALL_RECONCILE", err)
			}
			for _, p := range []string{victim, other, filepath.Join(home, AppIntentName)} {
				if _, err := os.Stat(p); err != nil {
					t.Fatalf("%s was removed: %v", p, err)
				}
			}
		})
	}
}

func TestAppReconcileExternalNonPathDest(t *testing.T) {
	requireAppIsolation(t)
	ctx := context.Background()
	t.Run("crash after RecordExternal", func(t *testing.T) {
		home, db := newAppHome(t)
		tx, err := BeginApp(ctx, db, home)
		if err != nil {
			t.Fatal(err)
		}
		err = tx.RecordExternal(ctx, AppPlacement{PackageID: appTestPkg, Version: "0.2.0", Format: metadata.ArtifactKindDeb, ArtifactSHA256: strings.Repeat("a", 64), Dest: "dpkg:festival-app"})
		if err != nil {
			t.Fatal(err)
		}
		abandon(tx)
		reopenAndClose(t, home, db)
		assertNoAppJournal(t, home)
		reopenAndClose(t, home, db)
	})
	t.Run("hand-written external entry with a non-path dest", func(t *testing.T) {
		home, _ := newAppHome(t)
		if err := writeAppIntent(home, AppIntent{Schema: 1, ID: "0123456789abcdef", Dest: "dpkg:festival-app", Phase: AppPhaseExternal}); err != nil {
			t.Fatal(err)
		}
		if err := ReconcileApp(ctx, home, nil); err != nil {
			t.Fatal(err)
		}
		assertNoAppJournal(t, home)
	})
	t.Run("external dest naming an existing dir is untouched", func(t *testing.T) {
		home, _ := newAppHome(t)
		dir := t.TempDir()
		keep := filepath.Join(dir, "keep.txt")
		if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeAppIntent(home, AppIntent{Schema: 1, ID: "0123456789abcdef", Dest: dir, Phase: AppPhaseExternal}); err != nil {
			t.Fatal(err)
		}
		if err := ReconcileApp(ctx, home, nil); err != nil {
			t.Fatal(err)
		}
		if b, err := os.ReadFile(keep); err != nil || string(b) != "keep" {
			t.Fatalf("external dest was touched: %q %v", b, err)
		}
		assertNoAppJournal(t, home)
	})
}

func TestAppExternalCommitAndClose(t *testing.T) {
	requireAppIsolation(t)
	ctx := context.Background()
	place := AppPlacement{PackageID: appTestPkg, Version: "0.2.0", Format: metadata.ArtifactKindDeb, ArtifactSHA256: strings.Repeat("b", 64), Dest: "dpkg:festival-app"}
	t.Run("commit with a good writer", func(t *testing.T) {
		home, db := newAppHome(t)
		tx, _ := BeginApp(ctx, db, home)
		if err := tx.RecordExternal(ctx, place); err != nil {
			t.Fatal(err)
		}
		calls := 0
		if _, err := tx.Commit(ctx, func(context.Context, *sql.DB) error { calls++; return nil }); err != nil || calls != 1 {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
		assertNoAppJournal(t, home)
		if err := tx.Close(ctx); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("commit with a failing writer", func(t *testing.T) {
		home, db := newAppHome(t)
		tx, _ := BeginApp(ctx, db, home)
		if err := tx.RecordExternal(ctx, place); err != nil {
			t.Fatal(err)
		}
		boom := errors.New("receipt failed")
		if _, err := tx.Commit(ctx, func(context.Context, *sql.DB) error { return boom }); !errors.Is(err, boom) {
			t.Fatalf("error = %v", err)
		}
		if err := tx.Close(ctx); err != nil {
			t.Fatal(err)
		}
		assertNoAppJournal(t, home)
		reopenAndClose(t, home, db)
	})
	t.Run("close without commit", func(t *testing.T) {
		home, db := newAppHome(t)
		tx, _ := BeginApp(ctx, db, home)
		if err := tx.RecordExternal(ctx, place); err != nil {
			t.Fatal(err)
		}
		if err := tx.Close(ctx); err != nil {
			t.Fatal(err)
		}
		assertNoAppJournal(t, home)
		reopenAndClose(t, home, db)
	})
	t.Run("order is enforced", func(t *testing.T) {
		home, db := newAppHome(t)
		parent := t.TempDir()
		tx, _ := BeginApp(ctx, db, home)
		if err := tx.RecordExternal(ctx, place); err != nil {
			t.Fatal(err)
		}
		_, err := tx.Prepare(ctx, AppPlacement{PackageID: appTestPkg, Version: "0.2.0", Format: metadata.ArtifactKindAppImage, ArtifactSHA256: "x", Dest: filepath.Join(parent, "festival-app")})
		if errpkg.Code(err) != "E_INSTALL_STAGING" {
			t.Fatalf("Prepare after RecordExternal: %v", err)
		}
		_ = tx.Close(ctx)

		tx, _ = BeginApp(ctx, db, home)
		if _, err := tx.Prepare(ctx, AppPlacement{PackageID: appTestPkg, Version: "0.2.0", Format: metadata.ArtifactKindAppImage, ArtifactSHA256: "x", Dest: filepath.Join(parent, "festival-app")}); err != nil {
			t.Fatal(err)
		}
		if err := tx.RecordExternal(ctx, place); errpkg.Code(err) != "E_INSTALL_STAGING" {
			t.Fatalf("RecordExternal after Prepare: %v", err)
		}
		_ = tx.Close(ctx)
	})
}

func TestAppReconcileStagingRemovesPartialStage(t *testing.T) {
	requireAppIsolation(t)
	ctx := context.Background()
	s := appScenario{metadata.ArtifactKindAppBundle, true}
	home, db := newAppHome(t)
	parent := t.TempDir()
	dest := s.dest(parent)
	seedOldApp(t, db, s, dest)
	tx, err := BeginApp(ctx, db, home)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := tx.Prepare(ctx, AppPlacement{PackageID: appTestPkg, Version: "0.1.8", Format: s.format, ArtifactSHA256: appTestNewSHA, Dest: dest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(payload, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	abandon(tx)
	reopenAndClose(t, home, db)
	if got := markerAt(t, dest); got != "old" {
		t.Fatalf("existing copy disturbed: %q", got)
	}
	assertNoSiblings(t, parent)
	assertNoAppJournal(t, home)
}
