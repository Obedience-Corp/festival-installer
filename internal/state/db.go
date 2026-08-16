package state

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

const (
	dbFilename    = "state.db"
	busyTimeoutMs = 5000
)

type DB struct {
	sql  *sql.DB
	path string
}

func OpenDB(ctx context.Context, home string) (*DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, errpkg.Wrap("E_DB_CTX", err, "context cancelled before open")
	}

	path := filepath.Join(home, dbFilename)
	dsn := buildDSN(path)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errpkg.Wrap("E_DB_OPEN", err, "cannot open "+path)
	}

	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, errpkg.Wrap("E_DB_PING", err, "cannot reach "+path)
	}

	if err := ensureWAL(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	db := &DB{sql: sqlDB, path: path}
	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func (d *DB) Close(ctx context.Context) error {
	if d == nil || d.sql == nil {
		return nil
	}
	if err := d.sql.Close(); err != nil {
		return errpkg.Wrap("E_DB_CLOSE", err, "cannot close "+d.path)
	}
	return nil
}

// Raw exposes the underlying *sql.DB for callers that need to compose with
// database/sql APIs (e.g. the receipts CRUD). Prefer Conn for new code.
func (d *DB) Raw() *sql.DB {
	return d.sql
}

func (d *DB) Conn(ctx context.Context) (*sql.Conn, error) {
	conn, err := d.sql.Conn(ctx)
	if err != nil {
		return nil, errpkg.Wrap("E_DB_CONN", err, "cannot acquire connection")
	}
	return conn, nil
}

func buildDSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeoutMs))
	q.Add("_pragma", "foreign_keys(ON)")
	return "file:" + path + "?" + q.Encode()
}

// ensureWAL sets journal_mode=WAL with retries. SQLite returns either an
// error or the current mode (silently!) when it can't switch due to a lock
// conflict, so this loop tolerates both kinds of failure under concurrent
// first-opens racing to convert a fresh DB.
func ensureWAL(ctx context.Context, db *sql.DB) error {
	const (
		maxAttempts = 50
		baseDelay   = 20 * time.Millisecond
	)
	var (
		mode    string
		lastErr error
	)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return errpkg.Wrap("E_DB_CTX", err, "context cancelled during WAL setup")
		}
		err := db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode)
		if err == nil && mode == "wal" {
			return nil
		}
		lastErr = err
		time.Sleep(baseDelay)
	}
	if lastErr != nil {
		return errpkg.Wrap("E_DB_PRAGMA", lastErr, "cannot set journal_mode after retries")
	}
	return errpkg.New("E_DB_WAL", "expected journal_mode=wal after retries, got "+mode)
}
