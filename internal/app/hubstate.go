package app

import (
	"context"
	"os"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// HubState reads a hub-owned value from FESTIVAL_HOME. A home with no database
// yet reports the key as absent rather than failing, so the hub can ask about
// its own state on a machine it has never written to. It never creates the home
// or the database, matching every other read path in this package.
func HubState(ctx context.Context, key string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, errpkg.Wrap("E_HUB_STATE_CTX", err, "context cancelled before reading hub state")
	}
	db, readable := openHomeDBIfExists(ctx)
	if db == nil {
		if !readable {
			return "", false, errpkg.New("E_HUB_STATE_HOME", "cannot read hub state from this home")
		}
		return "", false, nil
	}
	defer func() { _ = db.Close(ctx) }()
	return state.HubStateValue(ctx, db.Raw(), key)
}

// SetHubState writes a hub-owned value under the installer home lock, creating
// the home and the database when they do not exist yet. The lock is what keeps
// a hub write from racing an install that is running at the same time.
func SetHubState(ctx context.Context, key, value string) error {
	return state.WithHomeLock(ctx, func(ctx context.Context, db *state.DB) error {
		return state.SetHubState(ctx, db.Raw(), key, value)
	})
}

// DeleteHubState removes a hub-owned value under the installer home lock.
// A home with no database has nothing to delete and is not an error, so the
// hub can reset state it may never have written.
func DeleteHubState(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_HUB_STATE_CTX", err, "context cancelled before deleting hub state")
	}
	home, err := state.Home(ctx)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(state.DatabasePath(home)); os.IsNotExist(statErr) {
		return nil
	}
	return state.WithHomeLock(ctx, func(ctx context.Context, db *state.DB) error {
		return state.DeleteHubState(ctx, db.Raw(), key)
	})
}
