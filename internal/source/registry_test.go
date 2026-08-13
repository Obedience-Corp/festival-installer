package source_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
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

func sampleSource() source.Source {
	return source.Source{
		Name:    "official-obey",
		URL:     "https://github.com/Obedience-Corp/marketplace.git",
		Commit:  "abc123",
		AddedAt: time.Date(2026, 5, 22, 4, 0, 0, 0, time.UTC),
	}
}

func TestAddThenGet_RoundTrip(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	src := sampleSource()

	if err := source.Add(ctx, db.Raw(), src); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := source.Get(ctx, db.Raw(), src.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != src.Name || got.URL != src.URL || got.Commit != src.Commit {
		t.Fatalf("scalar mismatch: %+v", got)
	}
	if !got.AddedAt.Equal(src.AddedAt) {
		t.Fatalf("added_at mismatch: got %v want %v", got.AddedAt, src.AddedAt)
	}
}

func TestAdd_DuplicateRejected(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	src := sampleSource()

	if err := source.Add(ctx, db.Raw(), src); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := source.Add(ctx, db.Raw(), src); !errors.Is(err, source.ErrSourceExists) {
		t.Fatalf("expected ErrSourceExists, got %v", err)
	}
}

func TestList_OrderedByName(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()

	for _, name := range []string{"charlie", "alpha", "bravo"} {
		src := sampleSource()
		src.Name = name
		if err := source.Add(ctx, db.Raw(), src); err != nil {
			t.Fatalf("Add %s: %v", name, err)
		}
	}

	got, err := source.List(ctx, db.Raw())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Name != want[i] {
			t.Fatalf("order[%d]: got %q want %q", i, got[i].Name, want[i])
		}
	}
}

func TestGet_MissingReturnsNotFound(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	if _, err := source.Get(ctx, db.Raw(), "no-such-source"); !errors.Is(err, source.ErrSourceNotFound) {
		t.Fatalf("expected ErrSourceNotFound, got %v", err)
	}
}

func TestRemove_MissingReturnsNotFound(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	if err := source.Remove(ctx, db.Raw(), "no-such-source"); !errors.Is(err, source.ErrSourceNotFound) {
		t.Fatalf("expected ErrSourceNotFound, got %v", err)
	}
}

func TestRemove_DeletesSource(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	src := sampleSource()
	if err := source.Add(ctx, db.Raw(), src); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := source.Remove(ctx, db.Raw(), src.Name); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := source.Get(ctx, db.Raw(), src.Name); !errors.Is(err, source.ErrSourceNotFound) {
		t.Fatalf("expected ErrSourceNotFound after remove, got %v", err)
	}
}

func TestUpdateCommit(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	src := sampleSource()
	if err := source.Add(ctx, db.Raw(), src); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := source.UpdateCommit(ctx, db.Raw(), src.Name, "def456"); err != nil {
		t.Fatalf("UpdateCommit: %v", err)
	}
	got, err := source.Get(ctx, db.Raw(), src.Name)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Commit != "def456" {
		t.Fatalf("commit not updated: %s", got.Commit)
	}
	if err := source.UpdateCommit(ctx, db.Raw(), "no-such-source", "x"); !errors.Is(err, source.ErrSourceNotFound) {
		t.Fatalf("expected ErrSourceNotFound, got %v", err)
	}
}

func TestConcurrentAdd_Serializes(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()

	const n = 5
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := state.OpenDB(ctx, home)
			if err != nil {
				errs[i] = err
				return
			}
			defer func() { _ = db.Close(ctx) }()

			src := sampleSource()
			src.Name = fmt.Sprintf("pkg/%d", i)
			errs[i] = source.Add(ctx, db.Raw(), src)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	db, err := state.OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer func() { _ = db.Close(ctx) }()
	got, err := source.List(ctx, db.Raw())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != n {
		t.Fatalf("expected %d sources, got %d", n, len(got))
	}
}
