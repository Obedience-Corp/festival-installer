package receipts

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

var (
	ErrNotFound    = errpkg.New("E_RECEIPT_NOT_FOUND", "receipt not found")
	ErrInvalidPath = errpkg.New("E_RECEIPT_PATH", "owned file path must be absolute with no .. segments")
)

func Write(ctx context.Context, db *sql.DB, r Receipt) error {
	if err := validatePaths(r.OwnedFiles); err != nil {
		return err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return errpkg.Wrap("E_RECEIPT_CONN", err, "acquire conn for write")
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return errpkg.Wrap("E_RECEIPT_TX", err, "begin write tx")
	}

	rollback := func() { _, _ = conn.ExecContext(ctx, "ROLLBACK") }

	if _, err := conn.ExecContext(ctx,
		"DELETE FROM receipt_files WHERE receipt_pkg_id = ?", r.PackageID); err != nil {
		rollback()
		return errpkg.Wrap("E_RECEIPT_FILES_DEL", err, "clear owned files")
	}
	if _, err := conn.ExecContext(ctx,
		"DELETE FROM receipt_metadata WHERE receipt_pkg_id = ?", r.PackageID); err != nil {
		rollback()
		return errpkg.Wrap("E_RECEIPT_META_DEL", err, "clear metadata")
	}

	installedAt := r.InstalledAt
	if installedAt.IsZero() {
		installedAt = time.Now().UTC()
	}

	if _, err := conn.ExecContext(ctx,
		`INSERT INTO receipts(package_id, version, source, channel, installed_at)
		 VALUES(?, ?, ?, ?, ?)
		 ON CONFLICT(package_id) DO UPDATE SET
		   version=excluded.version,
		   source=excluded.source,
		   channel=excluded.channel,
		   installed_at=excluded.installed_at`,
		r.PackageID, r.Version, r.Source, r.Channel, installedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		rollback()
		return errpkg.Wrap("E_RECEIPT_UPSERT", err, "upsert receipt")
	}

	for _, f := range r.OwnedFiles {
		if _, err := conn.ExecContext(ctx,
			"INSERT INTO receipt_files(receipt_pkg_id, path, hash, mode) VALUES(?, ?, ?, ?)",
			r.PackageID, f.Path, f.Hash, int64(f.Mode),
		); err != nil {
			rollback()
			return errpkg.Wrap("E_RECEIPT_FILES_INS", err, "insert owned file")
		}
	}
	for k, v := range r.Metadata {
		if _, err := conn.ExecContext(ctx,
			"INSERT INTO receipt_metadata(receipt_pkg_id, key, value) VALUES(?, ?, ?)",
			r.PackageID, k, v,
		); err != nil {
			rollback()
			return errpkg.Wrap("E_RECEIPT_META_INS", err, "insert metadata")
		}
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return errpkg.Wrap("E_RECEIPT_COMMIT", err, "commit write")
	}
	return nil
}

func Get(ctx context.Context, db *sql.DB, packageID string) (Receipt, error) {
	var (
		r           Receipt
		installedAt string
	)
	err := db.QueryRowContext(ctx,
		"SELECT package_id, version, source, channel, installed_at FROM receipts WHERE package_id = ?",
		packageID,
	).Scan(&r.PackageID, &r.Version, &r.Source, &r.Channel, &installedAt)
	if err == sql.ErrNoRows {
		return Receipt{}, ErrNotFound
	}
	if err != nil {
		return Receipt{}, errpkg.Wrap("E_RECEIPT_GET", err, "select receipt")
	}
	t, err := time.Parse(time.RFC3339Nano, installedAt)
	if err != nil {
		return Receipt{}, errpkg.Wrap("E_RECEIPT_TIME", err, "parse installed_at")
	}
	r.InstalledAt = t

	files, err := loadFiles(ctx, db, packageID)
	if err != nil {
		return Receipt{}, err
	}
	r.OwnedFiles = files

	meta, err := loadMetadata(ctx, db, packageID)
	if err != nil {
		return Receipt{}, err
	}
	r.Metadata = meta

	return r, nil
}

func List(ctx context.Context, db *sql.DB, f Filter) ([]Receipt, error) {
	var (
		clauses []string
		args    []any
	)
	if f.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, f.Source)
	}
	if f.Version != "" {
		clauses = append(clauses, "version = ?")
		args = append(args, f.Version)
	}
	if !f.InstalledAfter.IsZero() {
		clauses = append(clauses, "installed_at > ?")
		args = append(args, f.InstalledAfter.UTC().Format(time.RFC3339Nano))
	}

	query := "SELECT package_id FROM receipts"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY package_id"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errpkg.Wrap("E_RECEIPT_LIST", err, "select package ids")
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, errpkg.Wrap("E_RECEIPT_LIST", err, "scan id")
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, errpkg.Wrap("E_RECEIPT_LIST", err, "iterate ids")
	}

	out := make([]Receipt, 0, len(ids))
	for _, id := range ids {
		r, err := Get(ctx, db, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func Delete(ctx context.Context, db *sql.DB, packageID string) error {
	res, err := db.ExecContext(ctx, "DELETE FROM receipts WHERE package_id = ?", packageID)
	if err != nil {
		return errpkg.Wrap("E_RECEIPT_DELETE", err, "delete receipt")
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return errpkg.Wrap("E_RECEIPT_DELETE", err, "rows affected")
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func loadFiles(ctx context.Context, db *sql.DB, packageID string) ([]OwnedFile, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT path, hash, mode FROM receipt_files WHERE receipt_pkg_id = ? ORDER BY path",
		packageID,
	)
	if err != nil {
		return nil, errpkg.Wrap("E_RECEIPT_FILES_LOAD", err, "select owned files")
	}
	defer func() { _ = rows.Close() }()

	var out []OwnedFile
	for rows.Next() {
		var (
			f    OwnedFile
			mode int64
		)
		if err := rows.Scan(&f.Path, &f.Hash, &mode); err != nil {
			return nil, errpkg.Wrap("E_RECEIPT_FILES_LOAD", err, "scan file")
		}
		f.Mode = osFileMode(mode)
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, errpkg.Wrap("E_RECEIPT_FILES_LOAD", err, "iterate files")
	}
	return out, nil
}

func loadMetadata(ctx context.Context, db *sql.DB, packageID string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT key, value FROM receipt_metadata WHERE receipt_pkg_id = ?",
		packageID,
	)
	if err != nil {
		return nil, errpkg.Wrap("E_RECEIPT_META_LOAD", err, "select metadata")
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, errpkg.Wrap("E_RECEIPT_META_LOAD", err, "scan meta")
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, errpkg.Wrap("E_RECEIPT_META_LOAD", err, "iterate meta")
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func validatePaths(files []OwnedFile) error {
	for _, f := range files {
		if !filepath.IsAbs(f.Path) {
			return ErrInvalidPath
		}
		for _, part := range strings.Split(filepath.ToSlash(f.Path), "/") {
			if part == ".." {
				return ErrInvalidPath
			}
		}
	}
	return nil
}
