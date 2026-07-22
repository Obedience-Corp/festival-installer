package gitsafe

import (
	"errors"
	"slices"
	"testing"
)

func TestValidateRemote(t *testing.T) {
	cases := []struct {
		name    string
		remote  string
		wantErr bool
	}{
		{"https ok", "https://github.com/o/r", false},
		{"https with creds ok", "https://user@github.com/o/r.git", false},
		{"ssh url ok", "ssh://git@host/o/r", false},
		{"ssh scp ok", "git@github.com:o/r.git", false},
		{"scp no user ok", "github.com:o/r", false},
		{"file url ok", "file:///srv/repo", false},
		{"absolute local path ok", "/tmp/bare-repo", false},
		{"absolute local path with colon ok", "/tmp/re:po", false},

		{"http rejected", "http://github.com/o/r", true},
		{"git rejected", "git://github.com/o/r", true},
		{"ftp rejected", "ftp://host/o/r", true},
		{"unknown scheme rejected", "gopher://host/o/r", true},
		{"leading dash", "--upload-pack=payload", true},
		{"ext transport", "ext::sh -c payload", true},
		{"remote helper form", "transport::address", true},
		{"empty", "", true},
		{"relative path rejected", "./local/repo", true},
		{"bare token rejected", "notaurl", true},
		{"empty host scp rejected", ":o/r", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRemote(tc.remote)
			if tc.wantErr && !errors.Is(err, ErrUnsafeRemote) {
				t.Fatalf("expected ErrUnsafeRemote for %q, got %v", tc.remote, err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.remote, err)
			}
		})
	}
}

func TestConfigArgsRestrictsTransports(t *testing.T) {
	args := ConfigArgs()
	if !slices.Contains(args, "protocol.ext.allow=never") {
		t.Fatalf("expected ext transport to be disabled, got %v", args)
	}
}
