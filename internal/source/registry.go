package source

import (
	"context"
	"database/sql"
	"time"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

var (
	ErrSourceExists   = errpkg.New("E_SOURCE_EXISTS", "source already exists")
	ErrSourceNotFound = errpkg.New("E_SOURCE_NOT_FOUND", "source not found")
)

func Add(ctx context.Context, db *sql.DB, src Source) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return errpkg.Wrap("E_SOURCE_CONN", err, "acquire conn for add")
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return errpkg.Wrap("E_SOURCE_TX", err, "begin add tx")
	}
	rollback := func() { _, _ = conn.ExecContext(ctx, "ROLLBACK") }

	var exists int
	err = conn.QueryRowContext(ctx,
		"SELECT 1 FROM sources WHERE name = ?", src.Name,
	).Scan(&exists)
	switch {
	case err == nil:
		rollback()
		return ErrSourceExists
	case err != sql.ErrNoRows:
		rollback()
		return errpkg.Wrap("E_SOURCE_EXISTS_CHECK", err, "check existing source")
	}

	addedAt := src.AddedAt
	if addedAt.IsZero() {
		addedAt = time.Now().UTC()
	}

	if _, err := conn.ExecContext(ctx,
		"INSERT INTO sources(name, url, commit_sha, added_at) VALUES(?, ?, ?, ?)",
		src.Name, src.URL, src.Commit, addedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		rollback()
		return errpkg.Wrap("E_SOURCE_INSERT", err, "insert source")
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return errpkg.Wrap("E_SOURCE_COMMIT", err, "commit add")
	}
	return nil
}

func Get(ctx context.Context, db *sql.DB, name string) (Source, error) {
	var (
		s       Source
		addedAt string
	)
	err := db.QueryRowContext(ctx,
		"SELECT name, url, commit_sha, added_at FROM sources WHERE name = ?", name,
	).Scan(&s.Name, &s.URL, &s.Commit, &addedAt)
	if err == sql.ErrNoRows {
		return Source{}, ErrSourceNotFound
	}
	if err != nil {
		return Source{}, errpkg.Wrap("E_SOURCE_GET", err, "select source")
	}
	t, err := time.Parse(time.RFC3339Nano, addedAt)
	if err != nil {
		return Source{}, errpkg.Wrap("E_SOURCE_TIME", err, "parse added_at")
	}
	s.AddedAt = t
	return s, nil
}

func List(ctx context.Context, db *sql.DB) ([]Source, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name, url, commit_sha, added_at FROM sources ORDER BY name",
	)
	if err != nil {
		return nil, errpkg.Wrap("E_SOURCE_LIST", err, "select sources")
	}
	defer func() { _ = rows.Close() }()

	var out []Source
	for rows.Next() {
		var (
			s       Source
			addedAt string
		)
		if err := rows.Scan(&s.Name, &s.URL, &s.Commit, &addedAt); err != nil {
			return nil, errpkg.Wrap("E_SOURCE_LIST", err, "scan source")
		}
		t, err := time.Parse(time.RFC3339Nano, addedAt)
		if err != nil {
			return nil, errpkg.Wrap("E_SOURCE_TIME", err, "parse added_at")
		}
		s.AddedAt = t
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, errpkg.Wrap("E_SOURCE_LIST", err, "iterate sources")
	}
	return out, nil
}

func UpdateCommit(ctx context.Context, db *sql.DB, name, commit string) error {
	res, err := db.ExecContext(ctx,
		"UPDATE sources SET commit_sha = ? WHERE name = ?", commit, name,
	)
	if err != nil {
		return errpkg.Wrap("E_SOURCE_UPDATE", err, "update commit")
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return errpkg.Wrap("E_SOURCE_UPDATE", err, "rows affected")
	}
	if affected == 0 {
		return ErrSourceNotFound
	}
	return nil
}

func Remove(ctx context.Context, db *sql.DB, name string) error {
	res, err := db.ExecContext(ctx, "DELETE FROM sources WHERE name = ?", name)
	if err != nil {
		return errpkg.Wrap("E_SOURCE_DELETE", err, "delete source")
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return errpkg.Wrap("E_SOURCE_DELETE", err, "rows affected")
	}
	if affected == 0 {
		return ErrSourceNotFound
	}
	return nil
}
