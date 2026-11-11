package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

const (
	appMetaFormat   = "app.format"
	appMetaLocation = "app.location"
	appMetaBundleID = "app.bundle_id"
	appMetaSHA      = "app.artifact_sha256"
	appMetaAdopted  = "app.adopted"
	appMetaManifest = "manifest_url"
)

type dbAppReceiptStore struct {
	db                  *sql.DB
	binDir, pluginsRoot string // pluginsRoot = ~/.obey/plugins
}

// appReceiptToRecord is the D006 shape: one OwnedFile with an empty hash and
// exactly six metadata keys.
func appReceiptToRecord(r appReceipt) receipts.Receipt {
	return receipts.Receipt{
		PackageID: AppPackageID, Version: r.Version, Source: r.Source, Channel: r.Channel,
		InstalledAt: time.Now().UTC(),
		OwnedFiles:  []receipts.OwnedFile{{Path: r.Executable, Hash: "", Mode: r.Mode}},
		Metadata: map[string]string{
			appMetaFormat: r.Format, appMetaLocation: r.Location, appMetaBundleID: r.BundleID,
			appMetaSHA: r.ArtifactSHA256, appMetaAdopted: boolString(r.Adopted), appMetaManifest: r.ManifestURL,
		},
	}
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func knownAppFormat(f string) bool {
	switch f {
	case metadata.ArtifactKindAppBundle, metadata.ArtifactKindNSIS, metadata.ArtifactKindDeb, metadata.ArtifactKindAppImage:
		return true
	}
	return false
}

func appReceiptFromRecord(rec receipts.Receipt) (appReceipt, error) {
	where := rec.Metadata[appMetaLocation]
	if where == "" {
		where = AppPackageID
	}
	if !knownAppFormat(rec.Metadata[appMetaFormat]) || len(rec.OwnedFiles) != 1 {
		return appReceipt{}, foreignAppError(where, "the receipt was not written by festival install app")
	}
	return appReceipt{
		Version: rec.Version, Source: rec.Source, Channel: rec.Channel, ManifestURL: rec.Metadata[appMetaManifest],
		Format: rec.Metadata[appMetaFormat], Location: rec.Metadata[appMetaLocation], BundleID: rec.Metadata[appMetaBundleID],
		ArtifactSHA256: rec.Metadata[appMetaSHA], Adopted: rec.Metadata[appMetaAdopted] == "true",
		Executable: rec.OwnedFiles[0].Path, Mode: rec.OwnedFiles[0].Mode,
	}, nil
}

func (s dbAppReceiptStore) Read(ctx context.Context) (*appReceipt, error) {
	rec, err := receipts.Get(ctx, s.db, AppPackageID)
	if errors.Is(err, receipts.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r, err := appReceiptFromRecord(rec)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func underRoot(path, root string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(resolvePath(root), resolvePath(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Write refuses an executable v0.3.12's TUI uninstall would delete unchecked.
func (s dbAppReceiptStore) Write(ctx context.Context, r appReceipt) error {
	if underRoot(r.Executable, s.binDir) || underRoot(r.Executable, s.pluginsRoot) {
		return errpkg.New("E_RECEIPT_PATH", "the Festival App executable "+r.Executable+" lies under a directory the released installer manages; refusing to record it")
	}
	return receipts.Write(ctx, s.db, appReceiptToRecord(r))
}

// productionAppReceipts returns the receipt store factory for this machine.
func productionAppReceipts(ctx context.Context) (func(*sql.DB) appReceiptStore, error) {
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return nil, err
	}
	plugins := ""
	if home, herr := os.UserHomeDir(); herr == nil {
		plugins = filepath.Join(home, ".obey", "plugins")
	}
	return func(db *sql.DB) appReceiptStore {
		return dbAppReceiptStore{db: db, binDir: binDir, pluginsRoot: plugins}
	}, nil
}

// readAppReceipt is the read-only path: it never creates the installer home.
func readAppReceipt(ctx context.Context) (*appReceipt, error) {
	home, err := state.Home(ctx)
	if err != nil {
		return nil, err
	}
	db, ok, err := state.OpenDBIfExists(ctx, home)
	if err != nil || !ok {
		return nil, err
	}
	defer func() { _ = db.Close(ctx) }()
	return dbAppReceiptStore{db: db.Raw()}.Read(ctx)
}
