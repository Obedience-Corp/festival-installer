package app

import (
	"context"
	"errors"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestAppResultUpdateResultMapping(t *testing.T) {
	cases := []struct {
		name string
		in   AppResult
		want UpdateResult
	}{
		{"changed", AppResult{Version: "0.2.1", PreviousVersion: "0.2.0", Changed: true}, UpdateResult{Package: AppPackageID, Action: "upgraded", Version: "0.2.1", From: "0.2.0"}},
		{"current", AppResult{Version: "0.2.1", PreviousVersion: "0.2.1"}, UpdateResult{Package: AppPackageID, Action: "current", Version: "0.2.1"}},
		{"absent", AppResult{}, UpdateResult{Package: AppPackageID, Action: "absent"}},
	}
	for _, tc := range cases {
		got := tc.in.updateResult()
		if got.Package != tc.want.Package || got.Action != tc.want.Action || got.Version != tc.want.Version || got.From != tc.want.From {
			t.Errorf("%s: got %+v want %+v", tc.name, got, tc.want)
		}
	}
}

func TestAppUpdateAllRunsBeforeFestival(t *testing.T) {
	checked := UpdatesResult{Packages: []PackageUpdate{
		{Package: FestivalPackageID, Installed: "0.2.9", Latest: "0.2.10", Status: UpdateAvailable},
		{Package: AppPackageID, Installed: "0.2.0", Latest: "0.2.1", Status: UpdateAvailable},
		{Package: "obedience-corp/camp-app", Installed: "1.0.0", Latest: "1.1.0", Status: UpdateAvailable},
	}}
	var order []string
	res := updateCheckedPackages(context.Background(), checked, func(_ context.Context, id string) (UpdateResult, string, error) {
		order = append(order, id)
		return UpdateResult{Package: id, Action: "upgraded"}, "", nil
	})
	if len(order) != 3 || order[2] != FestivalPackageID {
		t.Fatalf("order = %v, festival must be last", order)
	}
	seen := false
	for _, id := range order {
		seen = seen || id == AppPackageID
	}
	if !seen || res.Failed() {
		t.Fatalf("order=%v res=%+v", order, res)
	}
}

func TestAppUpdateAllRunningIsPartialFailure(t *testing.T) {
	checked := UpdatesResult{Packages: []PackageUpdate{
		{Package: AppPackageID, Installed: "0.2.0", Latest: "0.2.1", Status: UpdateAvailable},
		{Package: "obedience-corp/camp-app", Installed: "1.0.0", Latest: "1.1.0", Status: UpdateAvailable},
	}}
	res := updateCheckedPackages(context.Background(), checked, func(_ context.Context, id string) (UpdateResult, string, error) {
		if id == AppPackageID {
			return UpdateResult{}, "", errpkg.New("E_APP_RUNNING", "Festival App is running")
		}
		return UpdateResult{Package: id, Action: "upgraded"}, "", nil
	})
	if !res.Failed() || res.Packages[0].Action != "failed" || res.Packages[1].Action != "upgraded" {
		t.Fatalf("%+v", res)
	}
}

func TestAppCheckInstalledUpdatesMapsPlatformToUnsupported(t *testing.T) {
	entries := []ListEntry{{PackageID: AppPackageID, Version: "0.2.0", Channel: "stable", Source: "official-obey"}}
	res := checkInstalledUpdates(context.Background(), entries, func(context.Context, ListEntry) (PackageUpdate, error) {
		return PackageUpdate{}, errpkg.New("E_APP_PLATFORM", "no Festival App build for this host")
	})
	if res.Packages[0].Status != UpdateUnsupported {
		t.Fatalf("%+v", res.Packages[0])
	}
	res = checkInstalledUpdates(context.Background(), entries, func(context.Context, ListEntry) (PackageUpdate, error) {
		return PackageUpdate{}, errors.New("boom")
	})
	if res.Packages[0].Status != UpdateCheckFailed {
		t.Fatalf("%+v", res.Packages[0])
	}
}

func TestAppIsAppTarget(t *testing.T) {
	for _, yes := range []string{"app", "festival-app", "obedience-corp/festival-app"} {
		if !IsAppTarget(yes) {
			t.Errorf("%q must be an app target", yes)
		}
	}
	for _, no := range []string{"", "festival", "obey", "camp", "obedience-corp/camp-app"} {
		if IsAppTarget(no) {
			t.Errorf("%q must not be an app target", no)
		}
	}
}
