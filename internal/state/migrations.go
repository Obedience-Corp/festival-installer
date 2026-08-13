package state

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

var migrationNameRE = regexp.MustCompile(`^(\d{3,})_[a-z0-9_]+\.sql$`)

const migrationTable = `
CREATE TABLE IF NOT EXISTS _obey_installer_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TEXT NOT NULL
);`

type migration struct {
	version int
	name    string
	sql     string
}

func (d *DB) migrate(ctx context.Context) error {
	if _, err := d.sql.ExecContext(ctx, migrationTable); err != nil {
		return errpkg.Wrap("E_MIGRATE_BOOTSTRAP", err, "cannot create migration ledger")
	}

	all, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range all {
		if err := applyIfPending(ctx, d.sql, m); err != nil {
			return err
		}
	}
	return nil
}

func applyIfPending(ctx context.Context, db *sql.DB, m migration) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return errpkg.Wrap("E_MIGRATE_CONN", err, "acquire conn for "+m.name)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return errpkg.Wrap("E_MIGRATE_TX", err, "begin immediate for "+m.name)
	}

	var exists int
	err = conn.QueryRowContext(ctx,
		"SELECT 1 FROM _obey_installer_migrations WHERE version = ?", m.version,
	).Scan(&exists)
	if err == nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return nil
	}
	if err != sql.ErrNoRows {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return errpkg.Wrap("E_MIGRATE_LEDGER", err, "check applied for "+m.name)
	}

	if _, err := conn.ExecContext(ctx, m.sql); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return errpkg.Wrap("E_MIGRATE_APPLY", err, "apply "+m.name)
	}

	if _, err := conn.ExecContext(ctx,
		"INSERT INTO _obey_installer_migrations(version, name, applied_at) VALUES(?, ?, ?)",
		m.version, m.name, time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		return errpkg.Wrap("E_MIGRATE_RECORD", err, "record "+m.name)
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return errpkg.Wrap("E_MIGRATE_COMMIT", err, "commit "+m.name)
	}
	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, errpkg.Wrap("E_MIGRATE_READDIR", err, "cannot list embedded migrations")
	}

	out := make([]migration, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		match := migrationNameRE.FindStringSubmatch(e.Name())
		if match == nil {
			return nil, errpkg.New("E_MIGRATE_NAME", "bad migration filename: "+e.Name())
		}
		v, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, errpkg.Wrap("E_MIGRATE_NAME", err, "bad version in "+e.Name())
		}
		body, err := fs.ReadFile(migrationFS, "migrations/"+e.Name())
		if err != nil {
			return nil, errpkg.Wrap("E_MIGRATE_READ", err, "cannot read "+e.Name())
		}
		out = append(out, migration{version: v, name: e.Name(), sql: string(body)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })

	for i := 1; i < len(out); i++ {
		if out[i].version == out[i-1].version {
			return nil, errpkg.New("E_MIGRATE_DUP",
				"duplicate migration version: "+out[i].name+" and "+out[i-1].name)
		}
	}
	return out, nil
}
