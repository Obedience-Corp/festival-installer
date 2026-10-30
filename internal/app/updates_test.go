package app

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestUpdateAllPartialFailureAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checked := UpdatesResult{Packages: []PackageUpdate{
		{Package: FestivalPackageID, Status: UpdateAvailable},
		{Package: "one/camp-good", Status: UpdateAvailable},
		{Package: "one/camp-bad", Status: UpdateAvailable},
		{Package: "external", Status: UpdateExternal, Detail: "use brew"},
		{Package: "offline", Status: UpdateCheckFailed, Detail: "offline"},
	}}
	var calls []string
	result := updateCheckedPackages(ctx, checked, func(_ context.Context, id string) (UpdateResult, string, error) {
		calls = append(calls, id)
		if id == "one/camp-bad" {
			cancel()
			return UpdateResult{}, "", errors.New("checksum mismatch")
		}
		return UpdateResult{Package: id, Action: "upgraded", Version: "2.0.0"}, "", nil
	})
	if !reflect.DeepEqual(calls, []string{"one/camp-good", "one/camp-bad"}) {
		t.Fatalf("unexpected updates: %v", calls)
	}
	if !result.Failed() || result.Packages[0].Action != "upgraded" || result.Packages[1].Package != "one/camp-bad" || result.Packages[1].Error != "checksum mismatch" || result.Packages[2].Upgrade != "use brew" || result.Packages[4].Package != FestivalPackageID || result.Packages[4].Error != context.Canceled.Error() {
		t.Fatalf("partial results lost: %+v", result)
	}
}

func TestCheckUpdatesDoesNotCallOfflineCurrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entries := []ListEntry{{PackageID: "one", Version: "1.0.0"}, {PackageID: "two", Version: "1.0.0"}}
	result := checkInstalledUpdates(ctx, entries, func(_ context.Context, entry ListEntry) (PackageUpdate, error) {
		cancel()
		return PackageUpdate{Package: entry.PackageID, Installed: entry.Version}, errors.New("source offline")
	})
	if !result.Failed() || result.Available() != 0 || result.CheckedAt.IsZero() || result.Packages[1].Package != "two" || result.Packages[1].Detail != context.Canceled.Error() {
		t.Fatalf("bad check state: %+v", result)
	}
}
