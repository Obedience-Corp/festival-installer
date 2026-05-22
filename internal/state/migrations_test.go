package state

import (
	"context"
	"testing"
)

func TestMigrations_InitialAppliesOnce(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()

	for _, table := range []string{"receipts", "locks"} {
		var name string
		err := db.sql.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %q missing after migration: %v", table, err)
		}
	}

	var version int
	if err := db.sql.QueryRowContext(ctx,
		"SELECT version FROM _obey_installer_migrations ORDER BY version DESC LIMIT 1",
	).Scan(&version); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if version < 1 {
		t.Fatalf("expected applied version >= 1, got %d", version)
	}
}

func TestMigrations_OrderingByParsedVersion(t *testing.T) {
	all, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("expected at least one embedded migration")
	}
	for i := 1; i < len(all); i++ {
		if all[i].version <= all[i-1].version {
			t.Fatalf("migrations not strictly increasing: %d then %d (%s, %s)",
				all[i-1].version, all[i].version, all[i-1].name, all[i].name)
		}
	}
}
