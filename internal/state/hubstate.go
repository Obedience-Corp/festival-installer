package state

import (
	"context"
	"database/sql"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// Hub state keys. They live here so the TUI and the app layer address the same
// rows without repeating string literals.
const (
	// HubStateTourStep holds the getting-started tour's furthest completed
	// step, as a decimal string.
	HubStateTourStep = "tour.step"
	// HubStateTourDismissed records that the user closed the tour deliberately.
	HubStateTourDismissed = "tour.dismissed"
)

// HubStateValue reads a hub-owned state value. A missing key is not an error:
// callers get the zero value and ok false.
func HubStateValue(ctx context.Context, db *sql.DB, key string) (string, bool, error) {
	var value string
	err := db.QueryRowContext(ctx, "SELECT value FROM hub_state WHERE key = ?", key).Scan(&value)
	switch {
	case err == sql.ErrNoRows:
		return "", false, nil
	case err != nil:
		return "", false, errpkg.Wrap("E_HUB_STATE_GET", err, "read hub state "+key)
	default:
		return value, true, nil
	}
}

// SetHubState writes a hub-owned state value, replacing any existing one and
// moving updated_at.
func SetHubState(ctx context.Context, db *sql.DB, key, value string) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO hub_state(key, value, updated_at) VALUES(?, ?, ?) "+
			"ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at",
		key, value, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return errpkg.Wrap("E_HUB_STATE_SET", err, "write hub state "+key)
	}
	return nil
}

// DeleteHubState removes a key. Deleting a missing key is not an error.
func DeleteHubState(ctx context.Context, db *sql.DB, key string) error {
	if _, err := db.ExecContext(ctx, "DELETE FROM hub_state WHERE key = ?", key); err != nil {
		return errpkg.Wrap("E_HUB_STATE_DELETE", err, "delete hub state "+key)
	}
	return nil
}
