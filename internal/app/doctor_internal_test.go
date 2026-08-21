package app

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/source"
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
				{Name: "official-obey", Verified: false},
			},
			wantStatus: "fail",
			wantNames:  []string{"official-obey"},
		},
		{
			name: "official and third party both unverified: official wins",
			views: []source.ListView{
				{Name: "official-obey", Verified: false},
				{Name: "acme-plugins", Verified: false},
			},
			wantStatus: "fail",
			wantNames:  []string{"official-obey"},
		},
		{
			name: "third party only, unverified",
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
				{Name: "official-obey", Verified: true},
				{Name: "acme-plugins", Verified: true},
			},
			wantStatus: "ok",
			wantCount:  2,
		},
		{
			name: "official verified, third party unverified",
			views: []source.ListView{
				{Name: "official-obey", Verified: true},
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
			if tt.wantCount > 0 && !strings.Contains(got.Message, "2 source(s)") {
				t.Fatalf("Message %q does not report the verified count", got.Message)
			}
		})
	}
}
