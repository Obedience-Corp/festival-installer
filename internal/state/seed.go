package state

import (
	"context"
	"database/sql"
	"time"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
)

const OfficialSeedKey = "official-obey"

func SeedMarkerExists(ctx context.Context, db *sql.DB, key string) (bool, error) {
	var one int
	err := db.QueryRowContext(ctx, "SELECT 1 FROM seed_markers WHERE key = ?", key).Scan(&one)
	switch {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, errpkg.Wrap("E_SEED_GET", err, "check seed marker")
	default:
		return true, nil
	}
}

func RecordSeedMarker(ctx context.Context, db *sql.DB, key string) error {
	_, err := db.ExecContext(ctx,
		"INSERT OR IGNORE INTO seed_markers(key, seeded_at) VALUES(?, ?)",
		key, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return errpkg.Wrap("E_SEED_RECORD", err, "record seed marker")
	}
	return nil
}
