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
