package installer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

// JournalName is the stable install-intent file under installer home (TXN-01).
const JournalName = "install-intent.json"

// Journal records an in-flight activation so a crash mid-Commit can be reversed.
type Journal struct {
	PackageID string         `json:"package_id"`
	Version   string         `json:"version"`
	Channel   string         `json:"channel"`
	Source    string         `json:"source"`
	StartedAt time.Time      `json:"started_at"`
	Placed    []JournalPlace `json:"placed"`
}

// JournalPlace is one activated destination and optional pre-image backup path.
type JournalPlace struct {
	Dest   string `json:"dest"`
	Backup string `json:"backup,omitempty"`
}

func journalPath(home string) string {
	return filepath.Join(home, JournalName)
}

func writeJournal(home string, j Journal) error {
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "encode install intent")
	}
	path := journalPath(home)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "write install intent temp")
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "activate install intent")
	}
	return nil
}

func clearJournal(home string) error {
	err := os.Remove(journalPath(home))
	if err != nil && !os.IsNotExist(err) {
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "clear install intent")
	}
	return nil
}

func readJournal(home string) (Journal, bool, error) {
	raw, err := os.ReadFile(journalPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return Journal{}, false, nil
		}
		return Journal{}, false, errpkg.Wrap("E_INSTALL_JOURNAL", err, "read install intent")
	}
	var j Journal
	if err := json.Unmarshal(raw, &j); err != nil {
		return Journal{}, false, errpkg.Wrap("E_INSTALL_JOURNAL", err, "decode install intent")
	}
	return j, true, nil
}

// Reconcile reverses any incomplete install recorded in the intent journal.
// Call at process start (and before Begin) so a hard kill mid-Commit cannot leave
// a half-activated package without a receipt.
//
// If db is non-nil and a receipt already exists for the journaled package, the
// journal is treated as a stale success (clear only, no reverse).
func Reconcile(ctx context.Context, home string, db *sql.DB) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled before reconcile")
	}
	j, ok, err := readJournal(home)
	if err != nil || !ok {
		return err
	}
	if db != nil && j.PackageID != "" {
		if _, rerr := receipts.Get(ctx, db, j.PackageID); rerr == nil {
			return clearJournal(home)
		} else if !errors.Is(rerr, receipts.ErrNotFound) {
			return rerr
		}
	}
	// Reverse placements LIFO.
	for i := len(j.Placed) - 1; i >= 0; i-- {
		p := j.Placed[i]
		_ = os.Remove(p.Dest)
		if p.Backup != "" {
			_ = os.Rename(p.Backup, p.Dest)
		}
	}
	return clearJournal(home)
}
