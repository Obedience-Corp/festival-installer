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
		{"ssh scp ok", "git@github.com:o/r.git", false},
		{"ssh url ok", "ssh://git@host/o/r", false},
		{"local path ok", "/tmp/bare-repo", false},
		{"file url ok", "file:///srv/repo", false},
		{"leading dash", "--upload-pack=payload", true},
		{"ext transport", "ext::sh -c payload", true},
		{"empty", "", true},
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
