package state

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
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
	return "file:" + path + "?" + q.Encode()
}

// ensureWAL sets journal_mode=WAL with retries. SQLite returns the current
// mode (not an error) when it can't switch due to a lock conflict, so this
// loop tolerates concurrent first-opens racing to convert a fresh DB.
func ensureWAL(ctx context.Context, db *sql.DB) error {
	const (
		maxAttempts = 25
		baseDelay   = 20 * time.Millisecond
	)
	var mode string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return errpkg.Wrap("E_DB_CTX", err, "context cancelled during WAL setup")
		}
		if err := db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
			return errpkg.Wrap("E_DB_PRAGMA", err, "cannot set journal_mode")
		}
		if mode == "wal" {
			return nil
		}
		time.Sleep(baseDelay)
	}
	return errpkg.New("E_DB_WAL", "expected journal_mode=wal after retries, got "+mode)
}
