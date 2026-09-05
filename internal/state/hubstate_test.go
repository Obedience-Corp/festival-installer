package state

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestHubStateValue_Errors(t *testing.T) {
	db, _ := openTestDB(t)

	t.Run("cancelled context is reported as a coded error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := HubStateValue(ctx, db.Raw(), HubStateTourStep); err == nil {
			t.Fatal("expected an error reading with a cancelled context")
		} else if code := errpkg.Code(err); code != "E_HUB_STATE_GET" {
			t.Fatalf("code = %q, want E_HUB_STATE_GET", code)
		}
	})

	t.Run("cancelled context is reported when writing", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := SetHubState(ctx, db.Raw(), HubStateTourStep, "1"); err == nil {
			t.Fatal("expected an error writing with a cancelled context")
		} else if code := errpkg.Code(err); code != "E_HUB_STATE_SET" {
			t.Fatalf("code = %q, want E_HUB_STATE_SET", code)
		}
	})

	t.Run("cancelled context is reported when deleting", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := DeleteHubState(ctx, db.Raw(), HubStateTourStep); err == nil {
			t.Fatal("expected an error deleting with a cancelled context")
		} else if code := errpkg.Code(err); code != "E_HUB_STATE_DELETE" {
			t.Fatalf("code = %q, want E_HUB_STATE_DELETE", code)
		}
	})
}

func TestHubState_RoundTrip(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDB(t)

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "tour step", key: HubStateTourStep, value: "2"},
		{name: "tour dismissed", key: HubStateTourDismissed, value: "true"},
		{name: "empty value survives", key: "hub.empty", value: ""},
		{name: "multiline value survives", key: "hub.note", value: "one\ntwo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := SetHubState(ctx, db.Raw(), tc.key, tc.value); err != nil {
				t.Fatalf("SetHubState: %v", err)
			}
			got, ok, err := HubStateValue(ctx, db.Raw(), tc.key)
			if err != nil {
				t.Fatalf("HubStateValue: %v", err)
			}
			if !ok {
				t.Fatal("key not found after write")
			}
			if got != tc.value {
				t.Fatalf("value = %q, want %q", got, tc.value)
			}
		})
	}
}

func TestHubStateValue_MissingKeyIsNotAnError(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDB(t)

	got, ok, err := HubStateValue(ctx, db.Raw(), "hub.never-written")
	if err != nil {
		t.Fatalf("HubStateValue: %v", err)
	}
	if ok {
		t.Fatal("expected ok false for a key that was never written")
	}
	if got != "" {
		t.Fatalf("value = %q, want empty", got)
	}
}

func TestSetHubState_OverwritesInPlace(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDB(t)

	if err := SetHubState(ctx, db.Raw(), HubStateTourStep, "1"); err != nil {
		t.Fatalf("first SetHubState: %v", err)
	}
	first := hubStateUpdatedAt(t, db.Raw(), HubStateTourStep)

	time.Sleep(2 * time.Millisecond)
	if err := SetHubState(ctx, db.Raw(), HubStateTourStep, "3"); err != nil {
		t.Fatalf("second SetHubState: %v", err)
	}

	var rows int
	if err := db.Raw().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM hub_state WHERE key = ?", HubStateTourStep,
	).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d, want 1: the second write duplicated instead of replacing", rows)
	}

	got, _, err := HubStateValue(ctx, db.Raw(), HubStateTourStep)
	if err != nil {
		t.Fatalf("HubStateValue: %v", err)
	}
	if got != "3" {
		t.Fatalf("value = %q, want 3", got)
	}

	second := hubStateUpdatedAt(t, db.Raw(), HubStateTourStep)
	if !second.After(first) {
		t.Fatalf("updated_at did not move: first %s, second %s", first, second)
	}
}

func TestDeleteHubState(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDB(t)

	t.Run("deleting a missing key is not an error", func(t *testing.T) {
		if err := DeleteHubState(ctx, db.Raw(), "hub.never-written"); err != nil {
			t.Fatalf("DeleteHubState: %v", err)
		}
	})

	t.Run("deleting removes the value", func(t *testing.T) {
		if err := SetHubState(ctx, db.Raw(), HubStateTourDismissed, "true"); err != nil {
			t.Fatalf("SetHubState: %v", err)
		}
		if err := DeleteHubState(ctx, db.Raw(), HubStateTourDismissed); err != nil {
			t.Fatalf("DeleteHubState: %v", err)
		}
		if _, ok, err := HubStateValue(ctx, db.Raw(), HubStateTourDismissed); err != nil {
			t.Fatalf("HubStateValue: %v", err)
		} else if ok {
			t.Fatal("value still present after delete")
		}
	})
}

func hubStateUpdatedAt(t *testing.T, db *sql.DB, key string) time.Time {
	t.Helper()
	var raw string
	if err := db.QueryRowContext(context.Background(),
		"SELECT updated_at FROM hub_state WHERE key = ?", key,
	).Scan(&raw); err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t.Fatalf("updated_at %q is not RFC3339Nano, the format every other table uses: %v", raw, err)
	}
	return parsed
}

// TestHubStateMigration_UpgradeFromPreviousVersion is the test that catches real
// breakage: it builds a database at the schema version that shipped before
// hub_state, fills it with the rows a real home would have, and reopens it.
func TestHubStateMigration_UpgradeFromPreviousVersion(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	prior := hubStateMigrationVersion(t) - 1
	seedDatabaseAtVersion(t, filepath.Join(home, dbFilename), prior)

	db, err := OpenDB(ctx, home)
	if err != nil {
		t.Fatalf("reopen at current version: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })

	var name string
	if err := db.Raw().QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='hub_state'",
	).Scan(&name); err != nil {
		t.Fatalf("hub_state missing after upgrade: %v", err)
	}

	var receipts int
	if err := db.Raw().QueryRowContext(ctx, "SELECT COUNT(*) FROM receipts").Scan(&receipts); err != nil {
		t.Fatalf("count receipts: %v", err)
	}
	if receipts != 1 {
		t.Fatalf("receipts = %d, want 1: the upgrade lost installed-package rows", receipts)
	}

	var sources int
	if err := db.Raw().QueryRowContext(ctx, "SELECT COUNT(*) FROM sources").Scan(&sources); err != nil {
		t.Fatalf("count sources: %v", err)
	}
	if sources != 1 {
		t.Fatalf("sources = %d, want 1: the upgrade lost registered marketplaces", sources)
	}

	if err := SetHubState(ctx, db.Raw(), HubStateTourStep, "4"); err != nil {
		t.Fatalf("SetHubState on the upgraded database: %v", err)
	}
	got, ok, err := HubStateValue(ctx, db.Raw(), HubStateTourStep)
	if err != nil || !ok || got != "4" {
		t.Fatalf("round trip on upgraded database = (%q, %v, %v), want (4, true, nil)", got, ok, err)
	}
}

func TestHubStateMigration_AppliesToAFreshDatabase(t *testing.T) {
	ctx := context.Background()
	db, _ := openTestDB(t)

	var applied int
	if err := db.Raw().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM _obey_installer_migrations WHERE version = ?", hubStateMigrationVersion(t),
	).Scan(&applied); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if applied != 1 {
		t.Fatalf("hub_state migration recorded %d times, want 1", applied)
	}
}

func hubStateMigrationVersion(t *testing.T) int {
	t.Helper()
	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	for _, m := range all {
		if m.name == "005_hub_state.sql" {
			return m.version
		}
	}
	t.Fatal("005_hub_state.sql is not embedded")
	return 0
}

// seedDatabaseAtVersion applies every embedded migration up to and including
// maxVersion, then writes a receipt and a source, producing the database an
// existing user would be upgrading from.
func seedDatabaseAtVersion(t *testing.T, path string, maxVersion int) {
	t.Helper()
	ctx := context.Background()

	sqlDB, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		t.Fatalf("open prior-version database: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()
	sqlDB.SetMaxOpenConns(1)

	if _, err := sqlDB.ExecContext(ctx, migrationTable); err != nil {
		t.Fatalf("create migration ledger: %v", err)
	}

	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	applied := 0
	for _, m := range all {
		if m.version > maxVersion {
			continue
		}
		if _, err := sqlDB.ExecContext(ctx, m.sql); err != nil {
			t.Fatalf("apply %s: %v", m.name, err)
		}
		if _, err := sqlDB.ExecContext(ctx,
			"INSERT INTO _obey_installer_migrations(version, name, applied_at) VALUES(?, ?, ?)",
			m.version, m.name, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			t.Fatalf("record %s: %v", m.name, err)
		}
		applied++
	}
	if applied == 0 {
		t.Fatalf("no migrations at or below version %d", maxVersion)
	}

	if _, err := sqlDB.ExecContext(ctx,
		"INSERT INTO receipts(package_id, version, source, channel, installed_at) VALUES(?, ?, ?, ?, ?)",
		"festival", "0.1.0", "official-obey", "stable", time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed receipt: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx,
		"INSERT INTO sources(name, url, commit_sha, added_at) VALUES(?, ?, ?, ?)",
		"official-obey", "https://example.test/obey.git", "deadbeef", time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("seed source: %v", err)
	}
}
