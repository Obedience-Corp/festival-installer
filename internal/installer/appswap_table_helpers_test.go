//go:build (container_fs && linux) || (darwin && app_native)

package installer

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

const (
	appTestPkg    = "obedience-corp/festival-app"
	appTestNewSHA = "new-sha"
	appTestOldSHA = "old-sha"
)

var errCrash = errors.New("simulated crash")

type appScenario struct {
	format   string
	existing bool
}

func (s appScenario) name() string {
	n := "bundle"
	if s.format == metadata.ArtifactKindAppImage {
		n = "appimage"
	}
	if s.existing {
		return n + "-existing"
	}
	return n + "-fresh"
}

func (s appScenario) dest(parent string) string {
	if s.format == metadata.ArtifactKindAppImage {
		return filepath.Join(parent, "festival-app")
	}
	return filepath.Join(parent, "Festival App.app")
}

func newAppHome(t *testing.T) (string, *sql.DB) {
	t.Helper()
	home := t.TempDir()
	db, err := state.OpenDB(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return home, db.Raw()
}

func fakeBundle(t *testing.T, dir, marker string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Contents", "MacOS", "festival-app"), []byte(marker), 0o755); err != nil {
		t.Fatal(err)
	}
}

func fakeAppImage(t *testing.T, path, marker string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(marker), 0o755); err != nil {
		t.Fatal(err)
	}
}

func fillApp(t *testing.T, format, path, marker string) {
	t.Helper()
	if format == metadata.ArtifactKindAppImage {
		fakeAppImage(t, path, marker)
		return
	}
	fakeBundle(t, path, marker)
}

// markerAt returns the marker of the copy at dest, or "" when nothing is there.
func markerAt(t *testing.T, dest string) string {
	t.Helper()
	info, err := os.Lstat(dest)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	path := dest
	if info.IsDir() {
		path = filepath.Join(dest, "Contents", "MacOS", "festival-app")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertNoSiblings(t *testing.T, parent string) {
	t.Helper()
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".festival-") {
			t.Fatalf("sibling left behind: %s", filepath.Join(parent, e.Name()))
		}
	}
}

func isHex32(name string) bool {
	if len(name) != 32 {
		return false
	}
	for _, c := range name {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func assertNo32Hex(t *testing.T, roots ...string) {
	t.Helper()
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err == nil && isHex32(d.Name()) {
				t.Fatalf("32-hex name %s", path)
			}
			return nil
		})
	}
}

func assertNoAppJournal(t *testing.T, home string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(home, AppIntentName)); !os.IsNotExist(err) {
		t.Fatalf("app-intent.json still present: %v", err)
	}
}

// abandon models a killed process: the lock goes away, nothing is reconciled.
func abandon(tx *AppTxn) {
	_ = tx.release()
	tx.done = true
}

func appReceiptWriter(dest, version, sha string) func(context.Context, *sql.DB) error {
	return func(ctx context.Context, db *sql.DB) error {
		return receipts.Write(ctx, db, receipts.Receipt{
			PackageID: appTestPkg, Version: version, Source: "fixture", Channel: "stable",
			InstalledAt: time.Now().UTC(),
			OwnedFiles:  []receipts.OwnedFile{{Path: dest, Hash: "", Mode: 0o755}},
			Metadata:    map[string]string{"app.artifact_sha256": sha},
		})
	}
}

func crashAt(point string) func(string) error {
	return func(p string) error {
		if p == point {
			return errCrash
		}
		return nil
	}
}

func seedOldApp(t *testing.T, db *sql.DB, s appScenario, dest string) {
	t.Helper()
	if !s.existing {
		return
	}
	fillApp(t, s.format, dest, "old")
	if err := appReceiptWriter(dest, "0.1.7", appTestOldSHA)(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}

// stageApp runs Prepare, fills the payload with "new", MarkStaged and Swap.
func stageApp(ctx context.Context, t *testing.T, tx *AppTxn, s appScenario, dest string) error {
	t.Helper()
	payload, err := tx.Prepare(ctx, AppPlacement{PackageID: appTestPkg, Version: "0.1.8", Format: s.format, ArtifactSHA256: appTestNewSHA, Dest: dest})
	if err != nil {
		return err
	}
	fillApp(t, s.format, payload, "new")
	if err := tx.MarkStaged(ctx); err != nil {
		return err
	}
	return tx.Swap(ctx)
}

// runApp does a whole placement with hook; it returns the transaction and the first error.
func runApp(t *testing.T, home string, db *sql.DB, s appScenario, dest string, hook func(string) error) (*AppTxn, string, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := BeginApp(ctx, db, home)
	if err != nil {
		t.Fatalf("BeginApp: %v", err)
	}
	tx.afterPhase = hook
	if err := stageApp(ctx, t, tx, s, dest); err != nil {
		return tx, "", err
	}
	leftover, err := tx.Commit(ctx, appReceiptWriter(dest, "0.1.8", appTestNewSHA))
	return tx, leftover, err
}

func reopenAndClose(t *testing.T, home string, db *sql.DB) {
	t.Helper()
	tx, err := BeginApp(context.Background(), db, home)
	if err != nil {
		t.Fatalf("BeginApp after crash: %v", err)
	}
	if err := tx.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func assertReceipt(t *testing.T, db *sql.DB, wantVersion string) {
	t.Helper()
	r, err := receipts.Get(context.Background(), db, appTestPkg)
	if wantVersion == "" {
		if !errors.Is(err, receipts.ErrNotFound) {
			t.Fatalf("expected no receipt, got %+v %v", r, err)
		}
		return
	}
	if err != nil || r.Version != wantVersion {
		t.Fatalf("receipt = %+v %v, want version %s", r, err, wantVersion)
	}
}

func reachedPoints(t *testing.T, s appScenario) map[string]bool {
	t.Helper()
	home, db := newAppHome(t)
	parent := t.TempDir()
	dest := s.dest(parent)
	seedOldApp(t, db, s, dest)
	reached := map[string]bool{}
	tx, _, err := runApp(t, home, db, s, dest, func(p string) error { reached[p] = true; return nil })
	if err != nil {
		t.Fatalf("clean run: %v", err)
	}
	if err := tx.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := markerAt(t, dest); got != "new" {
		t.Fatalf("clean run marker %q", got)
	}
	return reached
}

// runCrashTable crashes a placement at every reached point and checks the next run.
func runCrashTable(t *testing.T, s appScenario) map[string]bool {
	t.Helper()
	reached := reachedPoints(t, s)
	for _, p := range []string{"staging-written", "staged-written", "swapping-written", "swapped-written", "receipt-written", "committed-written"} {
		if !reached[p] {
			t.Fatalf("%s: crash point %s never reached", s.name(), p)
		}
	}
	for point := range reached {
		t.Run(s.name()+"/"+point, func(t *testing.T) {
			home, db := newAppHome(t)
			parent := t.TempDir()
			dest := s.dest(parent)
			seedOldApp(t, db, s, dest)
			tx, _, err := runApp(t, home, db, s, dest, crashAt(point))
			if !errors.Is(err, errCrash) {
				t.Fatalf("expected a crash at %s, got %v", point, err)
			}
			if s.existing && s.format == metadata.ArtifactKindAppImage {
				if _, err := os.Lstat(dest); err != nil {
					t.Fatalf("file target went missing at %s: %v", point, err)
				}
			}
			abandon(tx)
			reopenAndClose(t, home, db)

			newState := point == "receipt-written" || point == "committed-written"
			wantMarker, wantVersion := "", ""
			switch {
			case newState:
				wantMarker, wantVersion = "new", "0.1.8"
			case s.existing:
				wantMarker, wantVersion = "old", "0.1.7"
			}
			if got := markerAt(t, dest); got != wantMarker {
				t.Fatalf("marker at %s after recovery = %q, want %q", point, got, wantMarker)
			}
			assertReceipt(t, db, wantVersion)
			assertNoSiblings(t, parent)
			assertNoAppJournal(t, home)
			assertNo32Hex(t, home, parent)
		})
	}
	return reached
}
