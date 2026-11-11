//go:build (container_fs && linux) || (darwin && app_native)

package installer

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
)

func TestAppSwapCrashRecovery(t *testing.T) {
	requireAppIsolation(t)
	for _, s := range []appScenario{
		{metadata.ArtifactKindAppBundle, true}, {metadata.ArtifactKindAppBundle, false},
		{metadata.ArtifactKindAppImage, true}, {metadata.ArtifactKindAppImage, false},
	} {
		reached := runCrashTable(t, s)
		switch {
		case s.format == metadata.ArtifactKindAppImage && s.existing && !reached["after-link"]:
			t.Fatalf("%s: after-link never reached", s.name())
		case s.format == metadata.ArtifactKindAppBundle && s.existing && !reached["after-exchange"] && !reached["after-first-rename"]:
			t.Fatalf("%s: no mid-swap point reached", s.name())
		}
	}
}

func TestAppSwapFileLinkFallback(t *testing.T) {
	requireAppIsolation(t)
	prev := linkFn
	linkFn = func(string, string) error { return syscall.EPERM }
	t.Cleanup(func() { linkFn = prev })
	s := appScenario{metadata.ArtifactKindAppImage, true}

	home, db := newAppHome(t)
	parent := t.TempDir()
	dest := s.dest(parent)
	seedOldApp(t, db, s, dest)
	tx, _, err := runApp(t, home, db, s, dest, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Close(context.Background())
	if markerAt(t, dest) != "new" {
		t.Fatalf("fallback swap did not install the new copy")
	}
	assertNoSiblings(t, parent)

	home, db = newAppHome(t)
	parent = t.TempDir()
	dest = s.dest(parent)
	seedOldApp(t, db, s, dest)
	tx, _, err = runApp(t, home, db, s, dest, crashAt("after-first-rename"))
	if !errors.Is(err, errCrash) {
		t.Fatalf("expected crash, got %v", err)
	}
	abandon(tx)
	reopenAndClose(t, home, db)
	if got := markerAt(t, dest); got != "old" {
		t.Fatalf("marker after fallback crash recovery = %q, want old", got)
	}
	assertNoSiblings(t, parent)
	assertNoAppJournal(t, home)
}

func fakeExchange(a, b string) error {
	tmp := a + ".swaptmp"
	for _, step := range [][2]string{{a, tmp}, {b, a}, {tmp, b}} {
		if err := os.Rename(step[0], step[1]); err != nil {
			return err
		}
	}
	return nil
}

func TestAppReconcileConvergesAfterInterruptedRestore(t *testing.T) {
	requireAppIsolation(t)
	prevExchange := exchangeFn
	t.Cleanup(func() { exchangeFn = prevExchange; appReconcileHook = nil })
	bundle := appScenario{metadata.ArtifactKindAppBundle, true}
	cases := []struct {
		name     string
		exchange func(string, string) error
		point    string
	}{
		{"simulated exchange", fakeExchange, "restore-after-exchange"},
		{"two renames", func(string, string) error { return errExchangeUnsupported }, "restore-after-first-rename"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exchangeFn = tc.exchange
			home, db := newAppHome(t)
			parent := t.TempDir()
			dest := bundle.dest(parent)
			seedOldApp(t, db, bundle, dest)
			tx, _, err := runApp(t, home, db, bundle, dest, crashAt("swapped-written"))
			if !errors.Is(err, errCrash) {
				t.Fatalf("expected crash, got %v", err)
			}
			abandon(tx)
			appReconcileHook = crashAt(tc.point)
			if _, err := BeginApp(context.Background(), db, home); !errors.Is(err, errCrash) {
				t.Fatalf("expected reconcile to crash at %s, got %v", tc.point, err)
			}
			appReconcileHook = nil
			reopenAndClose(t, home, db)
			if got := markerAt(t, dest); got != "old" {
				t.Fatalf("marker = %q, want old", got)
			}
			assertNoSiblings(t, parent)
			assertNoAppJournal(t, home)
		})
	}
	t.Run("appimage single rename", func(t *testing.T) {
		exchangeFn = prevExchange
		image := appScenario{metadata.ArtifactKindAppImage, true}
		home, db := newAppHome(t)
		parent := t.TempDir()
		dest := image.dest(parent)
		seedOldApp(t, db, image, dest)
		tx, _, err := runApp(t, home, db, image, dest, crashAt("swapped-written"))
		if !errors.Is(err, errCrash) {
			t.Fatalf("expected crash, got %v", err)
		}
		abandon(tx)
		reopenAndClose(t, home, db)
		if got := markerAt(t, dest); got != "old" {
			t.Fatalf("marker = %q, want old", got)
		}
		assertNoSiblings(t, parent)
	})
}

func TestAppSwapCommitRemovesOldCopy(t *testing.T) {
	requireAppIsolation(t)
	for _, s := range []appScenario{{metadata.ArtifactKindAppBundle, true}, {metadata.ArtifactKindAppImage, true}} {
		t.Run(s.name(), func(t *testing.T) {
			home, db := newAppHome(t)
			parent := t.TempDir()
			dest := s.dest(parent)
			seedOldApp(t, db, s, dest)
			tx, leftover, err := runApp(t, home, db, s, dest, nil)
			if err != nil || leftover != "" {
				t.Fatalf("commit: leftover %q err %v", leftover, err)
			}
			if err := tx.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
			if markerAt(t, dest) != "new" {
				t.Fatal("new copy not installed")
			}
			assertReceipt(t, db, "0.1.8")
			assertNoSiblings(t, parent)
			assertNoAppJournal(t, home)
		})
	}
}

func TestAppSwapCloseWithoutCommitRestores(t *testing.T) {
	requireAppIsolation(t)
	for _, s := range []appScenario{{metadata.ArtifactKindAppBundle, true}, {metadata.ArtifactKindAppImage, true}, {metadata.ArtifactKindAppBundle, false}} {
		t.Run(s.name(), func(t *testing.T) {
			ctx := context.Background()
			home, db := newAppHome(t)
			parent := t.TempDir()
			dest := s.dest(parent)
			seedOldApp(t, db, s, dest)
			tx, err := BeginApp(ctx, db, home)
			if err != nil {
				t.Fatal(err)
			}
			if err := stageApp(ctx, t, tx, s, dest); err != nil {
				t.Fatal(err)
			}
			if markerAt(t, dest) != "new" {
				t.Fatal("swap did not install the new copy")
			}
			if err := tx.Close(ctx); err != nil {
				t.Fatal(err)
			}
			want := ""
			if s.existing {
				want = "old"
			}
			if got := markerAt(t, dest); got != want {
				t.Fatalf("marker after Close = %q, want %q", got, want)
			}
			if err := tx.Close(ctx); err != nil {
				t.Fatalf("second Close: %v", err)
			}
			assertNoSiblings(t, parent)
			assertNoAppJournal(t, home)
		})
	}
}

func TestAppSwapReceiptFailureRestores(t *testing.T) {
	requireAppIsolation(t)
	ctx := context.Background()
	for _, s := range []appScenario{{metadata.ArtifactKindAppBundle, true}, {metadata.ArtifactKindAppImage, true}} {
		t.Run(s.name(), func(t *testing.T) {
			home, db := newAppHome(t)
			parent := t.TempDir()
			dest := s.dest(parent)
			seedOldApp(t, db, s, dest)
			tx, err := BeginApp(ctx, db, home)
			if err != nil {
				t.Fatal(err)
			}
			if err := stageApp(ctx, t, tx, s, dest); err != nil {
				t.Fatal(err)
			}
			boom := errors.New("receipt write failed")
			_, err = tx.Commit(ctx, func(context.Context, *sql.DB) error { return boom })
			if !errors.Is(err, boom) {
				t.Fatalf("Commit error = %v, want the receipt error", err)
			}
			if got := markerAt(t, dest); got != "old" {
				t.Fatalf("marker = %q, want old", got)
			}
			if err := tx.Close(ctx); err != nil {
				t.Fatal(err)
			}
			assertReceipt(t, db, "0.1.7")
			assertNoSiblings(t, parent)
			assertNoAppJournal(t, home)
		})
	}
}

func TestAppSwapLeftoverReportedAndRetried(t *testing.T) {
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
	if err := stageApp(ctx, t, tx, s, dest); err != nil {
		t.Fatal(err)
	}
	backup := tx.intent.Backup
	locked := filepath.Join(backup, "Contents", "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	leftover, err := tx.Commit(ctx, appReceiptWriter(dest, "0.1.8", appTestNewSHA))
	if err != nil || leftover != backup {
		t.Fatalf("Commit leftover %q err %v, want %q", leftover, err, backup)
	}
	j, ok, err := readAppIntent(home)
	if err != nil || !ok || j.Phase != AppPhaseCommitted {
		t.Fatalf("journal = %+v ok=%v err=%v, want phase committed", j, ok, err)
	}
	if err := tx.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	reopenAndClose(t, home, db)
	assertNoSiblings(t, parent)
	assertNoAppJournal(t, home)
	if markerAt(t, dest) != "new" {
		t.Fatal("new copy lost")
	}
}
