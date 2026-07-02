package artifacts

import (
	"errors"
	"testing"
)

func TestRequireHTTPS(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"https ok", "https://example.com/a.tar.gz", false},
		{"http rejected", "http://example.com/a.tar.gz", true},
		{"http loopback ok", "http://127.0.0.1:8080/a.tar.gz", false},
		{"http localhost ok", "http://localhost:8080/a.tar.gz", false},
		{"ftp rejected", "ftp://example.com/a", true},
		{"empty rejected", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := RequireHTTPS(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q", tc.url)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.url, err)
			}
			if tc.wantErr && !errors.Is(err, ErrInsecureURL) {
				t.Fatalf("expected ErrInsecureURL for %q, got %v", tc.url, err)
			}
		})
	}
}
