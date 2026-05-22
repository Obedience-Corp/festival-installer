package state

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"

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

	var mode string
	if err := sqlDB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		_ = sqlDB.Close()
		return nil, errpkg.Wrap("E_DB_PRAGMA", err, "cannot read journal_mode")
	}
	if mode != "wal" {
		_ = sqlDB.Close()
		return nil, errpkg.New("E_DB_WAL", "expected journal_mode=wal, got "+mode)
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

func (d *DB) migrate(ctx context.Context) error {
	return nil
}

func buildDSN(path string) string {
	q := url.Values{}
	q.Set("_journal", "WAL")
	q.Set("_busy_timeout", fmt.Sprintf("%d", busyTimeoutMs))
	return "file:" + path + "?" + q.Encode()
}
