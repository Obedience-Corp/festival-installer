package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/source"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

func TestMarketplaceTrustFrom(t *testing.T) {
	tests := []struct {
		name       string
		views      []source.ListView
		wantStatus string
		wantNames  []string
		wantCount  int
	}{
		{
			name: "official source unverified",
			views: []source.ListView{
				{Name: state.OfficialSeedKey, Verified: false},
			},
			wantStatus: "fail",
			wantNames:  []string{state.OfficialSeedKey},
		},
		{
			name: "official and third party both unverified: official wins",
			views: []source.ListView{
				{Name: state.OfficialSeedKey, Verified: false},
				{Name: "acme-plugins", Verified: false},
			},
			wantStatus: "fail",
			wantNames:  []string{state.OfficialSeedKey},
		},
		{
			name: "third party signature present but invalid: distinct fail, not lumped in with unsigned",
			views: []source.ListView{
				{Name: "acme-plugins", Verified: false, Err: "E_SIG_INVALID: signature failed verification"},
			},
			wantStatus: "fail",
			wantNames:  []string{"acme-plugins"},
		},
		{
			name: "third party signature invalid alongside a plain unsigned third party: invalid wins, unsigned does not mask it",
			views: []source.ListView{
				{Name: "acme-plugins", Verified: false, Err: "E_SIG_INVALID: signature failed verification"},
				{Name: "other-plugins", Verified: false},
			},
			wantStatus: "fail",
			wantNames:  []string{"acme-plugins"},
		},
		{
			name: "third party only, unverified, no signature at all",
			views: []source.ListView{
				{Name: "acme-plugins", Verified: false},
			},
			wantStatus: "warn",
			wantNames:  []string{"acme-plugins"},
		},
		{
			name:       "no sources registered",
			views:      nil,
			wantStatus: "warn",
			wantNames:  []string{"no marketplaces registered"},
		},
		{
			name: "official and third party both verified",
			views: []source.ListView{
				{Name: state.OfficialSeedKey, Verified: true},
				{Name: "acme-plugins", Verified: true},
			},
			wantStatus: "ok",
			wantCount:  2,
		},
		{
			name: "official verified, third party unverified",
			views: []source.ListView{
				{Name: state.OfficialSeedKey, Verified: true},
				{Name: "acme-plugins", Verified: false},
			},
			wantStatus: "warn",
			wantNames:  []string{"acme-plugins"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := marketplaceTrustFrom(tt.views)
			if got.ID != "marketplace_trust" {
				t.Fatalf("ID = %q, want marketplace_trust", got.ID)
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q (message: %q)", got.Status, tt.wantStatus, got.Message)
			}
			for _, name := range tt.wantNames {
				if !strings.Contains(got.Message, name) {
					t.Fatalf("Message %q does not mention %q", got.Message, name)
				}
			}
			if tt.wantCount > 0 {
				want := fmt.Sprintf("%d source(s)", tt.wantCount)
				if !strings.Contains(got.Message, want) {
					t.Fatalf("Message %q does not report the verified count %q", got.Message, want)
				}
			}
		})
	}
}

func TestIsSignatureError(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want bool
	}{
		{"empty", "", false},
		{"sig invalid", "E_SIG_INVALID: signature failed verification", true},
		{"sig malformed", "E_SIG_MALFORMED: missing key_id, algorithm, or signature", true},
		{"unsupported algorithm", "E_SIG_ALG: unsupported algorithm", true},
		{"context cancelled during verify", "E_SIG_CTX: context cancelled", true},
		{"unknown key id", "E_KEY_NOT_FOUND: verification key not found", true},
		{"sig file unreadable", "E_MARKETPLACE_SIG_READ: read obey-marketplace.json.sig", true},
		{"unrelated error, not a signature problem", "E_GIT_CLONE: could not read Username", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSignatureError(tt.err); got != tt.want {
				t.Fatalf("isSignatureError(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestManagedBinOnPathFrom covers the grading in isolation. The two rows that
// matter most sit next to each other: the same absent origin reads as pending on
// a first run and as fail once the home has been set up. If the relaxation ever
// leaks into the post-setup case, the "absent, past first run" rows fail here
// before any CLI test notices.
func TestManagedBinOnPathFrom(t *testing.T) {
	firstRun := SetupState{ManagedBin: "/home/u/.obey/installer/bin"}
	installed := SetupState{HasReceipts: true, ManagedBin: "/home/u/.obey/installer/bin"}
	registered := SetupState{HasMarketplaces: true, ManagedBin: "/home/u/.obey/installer/bin"}
	wired := SetupState{ManagedBinOnPath: true, ManagedBin: "/home/u/.obey/installer/bin"}

	tests := []struct {
		name       string
		origin     SuiteOrigin
		setup      SetupState
		wantStatus string
		wantSubstr string
	}{
		{"absent on a first run is pending", SuiteOrigin{Kind: OriginAbsent}, firstRun, DoctorPending, "festival shell-init"},
		{"absent with a receipt still fails", SuiteOrigin{Kind: OriginAbsent}, installed, DoctorFail, "no usable camp/fest/festival"},
		{"absent with a registered source still fails", SuiteOrigin{Kind: OriginAbsent}, registered, DoctorFail, "no usable camp/fest/festival"},
		{"absent with PATH already wired still fails", SuiteOrigin{Kind: OriginAbsent}, wired, DoctorFail, "no usable camp/fest/festival"},
		{
			"leftover binaries fail even on a first run",
			SuiteOrigin{Kind: OriginLeftover, Prefix: "/usr/local/bin"},
			firstRun,
			DoctorFail,
			"leftover camp/fest on PATH",
		},
		{
			"managed install is ok",
			SuiteOrigin{Kind: OriginManaged, Prefix: "/home/u/.obey/installer/bin"},
			installed,
			DoctorOK,
			"managed bin dir is on PATH",
		},
		{
			"managed install beside a package copy warns",
			SuiteOrigin{Kind: OriginManaged, Prefix: "/home/u/.obey/installer/bin", Dual: true},
			installed,
			DoctorWarn,
			"also a package copy",
		},
		{
			"package install is ok",
			SuiteOrigin{Kind: OriginPackage, Prefix: "/opt/homebrew/bin", Flavor: FlavorHomebrew},
			installed,
			DoctorOK,
			"suite on PATH via package",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := managedBinOnPathFrom(tt.origin, tt.setup)
			if got.ID != "managed_bin_on_path" {
				t.Fatalf("ID=%q", got.ID)
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("Status=%q, want %q (message %q)", got.Status, tt.wantStatus, got.Message)
			}
			if !strings.Contains(got.Message, tt.wantSubstr) {
				t.Fatalf("Message=%q, want it to contain %q", got.Message, tt.wantSubstr)
			}
		})
	}
}

func TestDoctorFailed_PendingIsNotAFailure(t *testing.T) {
	checks := []DoctorCheck{
		{ID: "managed_bin_on_path", Status: DoctorPending},
		{ID: "sources_reachable", Status: DoctorWarn},
		{ID: "receipts_integrity", Status: DoctorOK},
	}
	if DoctorFailed(checks) {
		t.Fatal("pending and warn must not set the failing exit code")
	}
	if !DoctorFailed(append(checks, DoctorCheck{ID: "path_shadowing", Status: DoctorFail})) {
		t.Fatal("a failing check must still set the failing exit code")
	}
}
