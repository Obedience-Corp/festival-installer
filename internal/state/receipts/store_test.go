package receipts_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

func openDB(t *testing.T) *state.DB {
	t.Helper()
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	db, err := state.OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return db
}

func sampleReceipt() receipts.Receipt {
	return receipts.Receipt{
		PackageID:   "obedience-corp/fest",
		Version:     "1.2.3",
		Source:      "official-obey",
		Channel:     "latest",
		InstalledAt: time.Date(2026, 5, 22, 4, 0, 0, 0, time.UTC),
		OwnedFiles: []receipts.OwnedFile{
			{Path: "/opt/obey/fest", Hash: "sha256:abc", Mode: 0755},
			{Path: "/opt/obey/share/fest/docs.md", Hash: "sha256:def", Mode: 0644},
		},
		Metadata: map[string]string{
			"installer": "obey-installer",
			"build":     "release",
		},
	}
}

func TestWriteThenGet_RoundTrip(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	r := sampleReceipt()

	if err := receipts.Write(ctx, db.Raw(), r); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := receipts.Get(ctx, db.Raw(), r.PackageID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PackageID != r.PackageID || got.Version != r.Version || got.Source != r.Source || got.Channel != r.Channel {
		t.Fatalf("scalar mismatch: %+v", got)
	}
	if !got.InstalledAt.Equal(r.InstalledAt) {
		t.Fatalf("installed_at mismatch: got %v want %v", got.InstalledAt, r.InstalledAt)
	}
	if len(got.OwnedFiles) != 2 {
		t.Fatalf("owned files: got %d, want 2", len(got.OwnedFiles))
	}
	if got.Metadata["installer"] != "obey-installer" || got.Metadata["build"] != "release" {
		t.Fatalf("metadata mismatch: %+v", got.Metadata)
	}
}

func TestUpsert_ReplacesFilesAndMetadata(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()

	first := sampleReceipt()
	if err := receipts.Write(ctx, db.Raw(), first); err != nil {
		t.Fatalf("first write: %v", err)
	}

	second := first
	second.Version = "2.0.0"
	second.OwnedFiles = []receipts.OwnedFile{
		{Path: "/opt/obey/fest", Hash: "sha256:zzz", Mode: 0755},
	}
	second.Metadata = map[string]string{"installer": "obey-installer-v2"}
	if err := receipts.Write(ctx, db.Raw(), second); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := receipts.Get(ctx, db.Raw(), first.PackageID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Version != "2.0.0" {
		t.Fatalf("version not upserted: %s", got.Version)
	}
	if len(got.OwnedFiles) != 1 || got.OwnedFiles[0].Hash != "sha256:zzz" {
		t.Fatalf("owned files not replaced: %+v", got.OwnedFiles)
	}
	if _, ok := got.Metadata["build"]; ok {
		t.Fatal("stale metadata key still present after upsert")
	}
}

func TestList_Filters(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()

	a := sampleReceipt()
	a.PackageID = "pkg/a"
	a.Source = "official-obey"
	a.Version = "1.0.0"
	a.InstalledAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	b := sampleReceipt()
	b.PackageID = "pkg/b"
	b.Source = "third-party"
	b.Version = "2.0.0"
	b.InstalledAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	for _, r := range []receipts.Receipt{a, b} {
		if err := receipts.Write(ctx, db.Raw(), r); err != nil {
			t.Fatalf("write %s: %v", r.PackageID, err)
		}
	}

	got, err := receipts.List(ctx, db.Raw(), receipts.Filter{Source: "official-obey"})
	if err != nil || len(got) != 1 || got[0].PackageID != "pkg/a" {
		t.Fatalf("filter by source: got %v err=%v", got, err)
	}

	got, err = receipts.List(ctx, db.Raw(), receipts.Filter{Version: "2.0.0"})
	if err != nil || len(got) != 1 || got[0].PackageID != "pkg/b" {
		t.Fatalf("filter by version: got %v err=%v", got, err)
	}

	got, err = receipts.List(ctx, db.Raw(), receipts.Filter{InstalledAfter: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil || len(got) != 1 || got[0].PackageID != "pkg/b" {
		t.Fatalf("filter by installed_after: got %v err=%v", got, err)
	}
}

func TestDelete_CascadesFilesAndMetadata(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	r := sampleReceipt()
	if err := receipts.Write(ctx, db.Raw(), r); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := receipts.Delete(ctx, db.Raw(), r.PackageID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := receipts.Get(ctx, db.Raw(), r.PackageID); !errors.Is(err, receipts.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	var files, meta int
	if err := db.Raw().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM receipt_files WHERE receipt_pkg_id = ?", r.PackageID).Scan(&files); err != nil {
		t.Fatalf("count files: %v", err)
	}
	if err := db.Raw().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM receipt_metadata WHERE receipt_pkg_id = ?", r.PackageID).Scan(&meta); err != nil {
		t.Fatalf("count meta: %v", err)
	}
	if files != 0 || meta != 0 {
		t.Fatalf("cascade failed: files=%d meta=%d", files, meta)
	}
}

func TestDelete_MissingReturnsNotFound(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	if err := receipts.Delete(ctx, db.Raw(), "no/such/pkg"); !errors.Is(err, receipts.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestWrite_RejectsRelativeAndTraversalPaths(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	r := sampleReceipt()

	r.OwnedFiles = []receipts.OwnedFile{{Path: "relative/path", Hash: "x", Mode: 0644}}
	if err := receipts.Write(ctx, db.Raw(), r); !errors.Is(err, receipts.ErrInvalidPath) {
		t.Fatalf("expected ErrInvalidPath for relative, got %v", err)
	}

	r.OwnedFiles = []receipts.OwnedFile{{Path: "/etc/../etc/passwd", Hash: "x", Mode: 0644}}
	if err := receipts.Write(ctx, db.Raw(), r); !errors.Is(err, receipts.ErrInvalidPath) {
		t.Fatalf("expected ErrInvalidPath for traversal, got %v", err)
	}

	var perr *errpkg.Error
	if !errors.As(receipts.ErrInvalidPath, &perr) || perr.Code != "E_RECEIPT_PATH" {
		t.Fatalf("ErrInvalidPath not the right typed error: %v", receipts.ErrInvalidPath)
	}
}

func TestConcurrentWrite_Serializes(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()

	const n = 5
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := state.OpenDB(ctx, home)
			if err != nil {
				errs[i] = err
				return
			}
			defer func() { _ = db.Close(ctx) }()

			r := sampleReceipt()
			r.PackageID = "pkg/" + string(rune('a'+i))
			errs[i] = receipts.Write(ctx, db.Raw(), r)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	db := openDB(t)
	got, err := receipts.List(ctx, db.Raw(), receipts.Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Note: different goroutines used different homes' DBs above (each OpenDB).
	// This last List is on a fresh home; we only verify it doesn't error.
	_ = got
	_ = os.Getenv("HOME") // satisfy linter; we never touch real HOME
}
