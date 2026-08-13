package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func openTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	db, err := OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	return db, home
}

func TestOpenDB_FreshOpenCreatesFileAndEnablesWAL(t *testing.T) {
	db, home := openTestDB(t)

	if _, err := os.Stat(filepath.Join(home, "state.db")); err != nil {
		t.Fatalf("state.db not created: %v", err)
	}

	var mode string
	if err := db.sql.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func TestOpenDB_ReopenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	db1, err := OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("first OpenDB: %v", err)
	}
	if err := db1.Close(ctx); err != nil {
		t.Fatalf("close db1: %v", err)
	}

	db2, err := OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("second OpenDB: %v", err)
	}
	t.Cleanup(func() { _ = db2.Close(ctx) })

	var count int
	row := db2.sql.QueryRow("SELECT COUNT(*) FROM _obey_installer_migrations")
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one applied migration on reopen")
	}
}

func TestOpenDB_CorruptFileReturnsTypedError(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "state.db"), []byte("not a sqlite file"), 0600); err != nil {
		t.Fatalf("seed corrupt db: %v", err)
	}

	_, err := OpenDB(context.Background(), home)
	if err == nil {
		t.Fatal("expected error opening corrupt db")
	}
	var perr *errpkg.Error
	if !errors.As(err, &perr) {
		t.Fatalf("error is not *errpkg.Error: %T (%v)", err, err)
	}
	switch perr.Code {
	case "E_DB_PING", "E_DB_PRAGMA", "E_DB_WAL", "E_MIGRATE_BOOTSTRAP", "E_MIGRATE_LEDGER":
	default:
		t.Fatalf("unexpected error code %q (%v)", perr.Code, err)
	}
}

func TestOpenDB_ConcurrentOpenSerializes(t *testing.T) {
	home := t.TempDir()
	ctx := context.Background()

	const n = 5
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := OpenDB(ctx, home)
			if err != nil {
				errs[i] = err
				return
			}
			defer func() { _ = db.Close(ctx) }()

			conn, err := db.Conn(ctx)
			if err != nil {
				errs[i] = err
				return
			}
			_ = conn.Close()
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}
}

func TestDB_ConnReturnsUsableConnection(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer func() { _ = conn.Close() }()

	var one int
	if err := conn.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 returned %d", one)
	}
}

func TestDB_CloseIsIdempotent(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()

	if err := db.Close(ctx); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := db.Close(ctx); err != nil {
		t.Fatalf("second close: %v", err)
	}

	if _, err := db.Conn(ctx); err == nil {
		t.Fatal("Conn after Close should fail")
	}
}
