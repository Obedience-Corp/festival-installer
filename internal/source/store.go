package source

import (
	"context"
	"database/sql"
	"time"

	insterr "github.com/Obedience-Corp/obey-installer/internal/errors"
)

var (
	ErrNotFound  = insterr.Sentinel("source not found")
	ErrDuplicate = insterr.Sentinel("source already exists")
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Add(ctx context.Context, src Source) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return insterr.Wrap(err, "begin tx")
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO sources (name, url, commit_sha, added_at) VALUES (?, ?, ?, ?)`,
		src.Name, src.URL, src.Commit, src.AddedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return insterr.Wrap(err, "insert source")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return insterr.Wrap(err, "rows affected")
	}
	if n == 0 {
		return ErrDuplicate
	}
	if err := tx.Commit(); err != nil {
		return insterr.Wrap(err, "commit tx")
	}
	return nil
}

func (s *Store) List(ctx context.Context) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT name, url, commit_sha, added_at FROM sources ORDER BY name`,
	)
	if err != nil {
		return nil, insterr.Wrap(err, "query sources")
	}
	defer rows.Close()

	var out []Source
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	if err := rows.Err(); err != nil {
		return nil, insterr.Wrap(err, "iterate sources")
	}
	return out, nil
}

func (s *Store) Get(ctx context.Context, name string) (Source, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT name, url, commit_sha, added_at FROM sources WHERE name = ?`, name,
	)
	src, err := scanSource(row)
	if err != nil {
		if insterr.Is(err, sql.ErrNoRows) {
			return Source{}, ErrNotFound
		}
		return Source{}, err
	}
	return src, nil
}

func (s *Store) UpdateCommit(ctx context.Context, name, commit string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return insterr.Wrap(err, "begin tx")
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE sources SET commit_sha = ? WHERE name = ?`, commit, name,
	)
	if err != nil {
		return insterr.Wrap(err, "update commit")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return insterr.Wrap(err, "rows affected")
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return insterr.Wrap(err, "commit tx")
	}
	return nil
}

func (s *Store) Remove(ctx context.Context, name string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return insterr.Wrap(err, "begin tx")
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM sources WHERE name = ?`, name)
	if err != nil {
		return insterr.Wrap(err, "delete source")
	}
	n, err := res.RowsAffected()
	if err != nil {
		return insterr.Wrap(err, "rows affected")
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return insterr.Wrap(err, "commit tx")
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSource(sc scanner) (Source, error) {
	var (
		src     Source
		addedAt string
	)
	if err := sc.Scan(&src.Name, &src.URL, &src.Commit, &addedAt); err != nil {
		return Source{}, insterr.Wrap(err, "scan source")
	}
	t, err := time.Parse(time.RFC3339, addedAt)
	if err != nil {
		return Source{}, insterr.Wrap(err, "parse added_at")
	}
	src.AddedAt = t
	return src, nil
}
