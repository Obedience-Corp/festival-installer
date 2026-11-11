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
	"regexp"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

// AppIntentName is the Festival App journal under installer home. It is separate
// from install-intent.json, which released installers share.
const AppIntentName = "app-intent.json"

type AppPhase string

const (
	AppPhaseStaging   AppPhase = "staging"   // journal written; stage dir being filled
	AppPhaseStaged    AppPhase = "staged"    // payload complete and checked; identity recorded
	AppPhaseSwapping  AppPhase = "swapping"  // written immediately before the first rename
	AppPhaseSwapped   AppPhase = "swapped"   // dest holds the new copy; old copy at Old (or none)
	AppPhaseCommitted AppPhase = "committed" // receipt durable
	AppPhaseExternal  AppPhase = "external"  // nsis or deb: the OS installer owns atomicity
)

type AppIntent struct {
	Schema         int       `json:"schema"` // 1
	ID             string    `json:"id"`     // 16 lowercase hex
	PackageID      string    `json:"package_id"`
	Version        string    `json:"version"`
	Format         string    `json:"format"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	Dest           string    `json:"dest"`
	Staged         string    `json:"staged"`  // the stage dir
	Payload        string    `json:"payload"` // <staged>/<base(dest)>
	Backup         string    `json:"backup"`  // the old-copy sibling, "" for a fresh install
	PayloadDev     uint64    `json:"payload_dev,omitempty"`
	PayloadIno     uint64    `json:"payload_ino,omitempty"`
	Phase          AppPhase  `json:"phase"`
	StartedAt      time.Time `json:"started_at"`
}

var appIntentIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// appReconcileHook is a test hook called at named points inside the restore
// path. A non-nil return makes ReconcileApp stop with that error, journal kept.
var appReconcileHook func(point string) error

func newAppIntentID() (string, error) {
	var raw [8]byte
	if _, err := crand.Read(raw[:]); err != nil {
		return "", errpkg.Wrap("E_INSTALL_JOURNAL", err, "generate app intent id")
	}
	return hex.EncodeToString(raw[:]), nil
}

func stagedSiblingPath(dest, id string) string {
	return filepath.Join(filepath.Dir(dest), "."+filepath.Base(dest)+".festival-new-"+id)
}

func oldSiblingPath(dest, id string) string {
	return filepath.Join(filepath.Dir(dest), "."+filepath.Base(dest)+".festival-old-"+id)
}

func appIntentPath(home string) string { return filepath.Join(home, AppIntentName) }

func writeAppIntent(home string, j AppIntent) error {
	raw, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "encode app intent")
	}
	path := appIntentPath(home)
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "write app intent temp")
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "write app intent temp")
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "sync app intent temp")
	}
	if err := f.Close(); err != nil {
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "close app intent temp")
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "activate app intent")
	}
	return nil
}

func readAppIntent(home string) (AppIntent, bool, error) {
	raw, err := os.ReadFile(appIntentPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return AppIntent{}, false, nil
		}
		return AppIntent{}, false, errpkg.Wrap("E_INSTALL_JOURNAL", err, "read app intent")
	}
	var j AppIntent
	if err := json.Unmarshal(raw, &j); err != nil {
		return AppIntent{}, false, errpkg.Wrap("E_INSTALL_JOURNAL", err, "decode app intent")
	}
	return j, true, nil
}

func clearAppIntent(home string) error {
	if err := os.Remove(appIntentPath(home)); err != nil && !os.IsNotExist(err) {
		return errpkg.Wrap("E_INSTALL_JOURNAL", err, "clear app intent")
	}
	return nil
}

func validAppPhase(p AppPhase) bool {
	switch p {
	case AppPhaseStaging, AppPhaseStaged, AppPhaseSwapping, AppPhaseSwapped, AppPhaseCommitted, AppPhaseExternal:
		return true
	}
	return false
}

// validateAppIntent refuses a journal whose paths are not this design's
// siblings, so a forged or corrupt file cannot make reconcile delete anything.
func validateAppIntent(j AppIntent) error {
	if !appIntentIDPattern.MatchString(j.ID) || !validAppPhase(j.Phase) {
		return errpkg.New("E_INSTALL_RECONCILE", "app-intent.json has an unknown id or phase; left it alone: id="+j.ID+" phase="+string(j.Phase))
	}
	if j.Phase == AppPhaseExternal {
		return nil
	}
	ok := filepath.IsAbs(j.Dest) && filepath.Clean(j.Dest) == j.Dest && filepath.Dir(j.Dest) != j.Dest &&
		j.Staged == stagedSiblingPath(j.Dest, j.ID) &&
		j.Payload == filepath.Join(j.Staged, filepath.Base(j.Dest)) &&
		(j.Backup == "" || j.Backup == oldSiblingPath(j.Dest, j.ID))
	if !ok {
		return errpkg.New("E_INSTALL_RECONCILE", "app-intent.json names unexpected paths; left them alone: dest="+j.Dest+" staged="+j.Staged+" payload="+j.Payload+" backup="+j.Backup)
	}
	return nil
}

func appReconcileStep(point string) error {
	if appReconcileHook != nil {
		return appReconcileHook(point)
	}
	return nil
}

func appReceiptCommitted(ctx context.Context, db *sql.DB, j AppIntent) (bool, error) {
	if db == nil {
		return false, nil
	}
	r, err := receipts.Get(ctx, db, j.PackageID)
	if errors.Is(err, receipts.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return r.Version == j.Version && r.Metadata["app.artifact_sha256"] == j.ArtifactSHA256, nil
}
