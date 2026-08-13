package installer

import (
	"context"
	crand "crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// JournalName is the stable install-intent file under installer home (TXN-01).
const JournalName = "install-intent.json"

// IntentBackupsDir holds durable pre-images for in-flight activations.
// Kept outside the per-transaction staging temp dir so finish()/crash cleanup
// cannot delete the only recovery copies.
const IntentBackupsDir = "intent-backups"

// Journal records an in-flight activation so a crash mid-Commit can be reversed.
type Journal struct {
	ID        string         `json:"id"`
	PackageID string         `json:"package_id"`
	Version   string         `json:"version"`
	Channel   string         `json:"channel"`
	Source    string         `json:"source"`
	StartedAt time.Time      `json:"started_at"`
	Placed    []JournalPlace `json:"placed"`
}

func newJournalID() (string, error) {
	var raw [16]byte
	if _, err := crand.Read(raw[:]); err != nil {
		return "", errpkg.Wrap("E_INSTALL_JOURNAL", err, "generate install intent id")
	}
	return hex.EncodeToString(raw[:]), nil
}

// JournalPlace is one activated destination and optional pre-image backup path.
type JournalPlace struct {
	Dest   string `json:"dest"`
	Backup string `json:"backup,omitempty"`
}

func journalPath(home string) string {
	return filepath.Join(home, JournalName)
}

func intentBackupDir(home, journalID string) string {
	return filepath.Join(home, IntentBackupsDir, journalID)
}

func writeJournal(home string, j Journal) error {
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "encode install intent")
	}
	path := journalPath(home)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "write install intent temp")
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "write install intent temp")
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "sync install intent temp")
	}
	if err := f.Close(); err != nil {
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "close install intent temp")
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

func clearIntentBackups(home, journalID string) {
	if journalID == "" {
		return
	}
	_ = os.RemoveAll(intentBackupDir(home, journalID))
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
// The caller must hold the install lock so recovery cannot race a live commit.
// Call before starting a new activation so a hard kill mid-Commit cannot leave
// a half-activated package without a receipt.
//
// If db is non-nil and a receipt exists with the same transaction ID as the
// journal, the journal is treated as a completed success (clear only, no
// reverse). An older receipt for the same package is not sufficient.
func Reconcile(ctx context.Context, home string, db *sql.DB) error {
	if err := ctx.Err(); err != nil {
		return errpkg.Wrap("E_INSTALL_CTX", err, "context cancelled before reconcile")
	}
	j, ok, err := readJournal(home)
	if err != nil || !ok {
		return err
	}
	if db != nil && j.PackageID != "" {
		if r, rerr := receipts.Get(ctx, db, j.PackageID); rerr == nil {
			if j.ID != "" && r.Metadata["install_journal_id"] == j.ID {
				clearIntentBackups(home, j.ID)
				return clearJournal(home)
			}
		} else if !errors.Is(rerr, receipts.ErrNotFound) {
			return rerr
		}
	}
	// Reverse placements LIFO. Persist each successful removal so a later
	// failure does not cause already-repaired entries to be retried forever.
	var reconcileErr error
	for i := len(j.Placed) - 1; i >= 0; i-- {
		p := j.Placed[i]
		if err := removePlacement(p); err != nil {
			reconcileErr = errors.Join(reconcileErr, err)
			continue
		}
		j.Placed = append(j.Placed[:i], j.Placed[i+1:]...)
		if err := writeJournal(home, j); err != nil {
			reconcileErr = errors.Join(reconcileErr, err)
			break
		}
	}
	if reconcileErr != nil {
		return errpkg.Wrap("E_INSTALL_RECONCILE", reconcileErr, "reverse incomplete install")
	}
	clearIntentBackups(home, j.ID)
	return clearJournal(home)
}

// removePlacement reverses one journal entry based on observed filesystem state.
//
// States after a kill mid-activation:
//   - Backup path set, backup missing: not started (leave Dest alone).
//   - Backup path set, backup present: remove Dest if present, restore backup.
//   - Backup empty: fresh install; remove Dest if present.
func removePlacement(p JournalPlace) error {
	if p.Backup != "" {
		_, berr := os.Lstat(p.Backup)
		if berr != nil {
			if os.IsNotExist(berr) {
				// Journaled before backupExisting completed — live Dest is still
				// the previous binary (or absent). Do not delete it.
				return nil
			}
			return errpkg.Wrap("E_INSTALL_RECONCILE", berr, "inspect backup "+p.Backup)
		}
		if err := os.Remove(p.Dest); err != nil && !os.IsNotExist(err) {
			return errpkg.Wrap("E_INSTALL_RECONCILE", err, "remove incomplete placement "+p.Dest)
		}
		if err := os.Rename(p.Backup, p.Dest); err != nil {
			return errpkg.Wrap("E_INSTALL_RECONCILE", err, "restore backup "+p.Backup)
		}
		return nil
	}
	if err := os.Remove(p.Dest); err != nil && !os.IsNotExist(err) {
		return errpkg.Wrap("E_INSTALL_RECONCILE", err, "remove incomplete placement "+p.Dest)
	}
	return nil
}
