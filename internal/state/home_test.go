package state

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

func TestHome(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		wantCode string
		wantSub  string
	}{
		{"env override absolute", "/tmp/obey-test", "", "/tmp/obey-test"},
		{"env override empty falls back to userHome", "", "", ".obey/installer"},
		{"env override non-absolute rejected", "relative/path", "E_HOME_NOT_ABS", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FESTIVAL_HOME", "")
			t.Setenv("OBEY_INSTALLER_HOME", tt.envValue)
			got, err := Home(context.Background())
			if tt.wantCode != "" {
				var e *errpkg.Error
				if !errors.As(err, &e) || e.Code != tt.wantCode {
					t.Fatalf("want code %q, got err %v", tt.wantCode, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantSub != "" && !strings.Contains(got, tt.wantSub) {
				t.Fatalf("want path to contain %q, got %q", tt.wantSub, got)
			}
		})
	}
}

func TestHome_FestivalEnvPreferred(t *testing.T) {
	t.Setenv("FESTIVAL_HOME", "/tmp/festival-home-pref")
	t.Setenv("OBEY_INSTALLER_HOME", "/tmp/festival-installer-home")
	got, err := Home(context.Background())
	if err != nil {
		t.Fatalf("Home: %v", err)
	}
	if got != "/tmp/festival-home-pref" {
		t.Fatalf("want FESTIVAL_HOME to win, got %q", got)
	}
}

func TestEnsureHome(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "installer")
	t.Setenv("FESTIVAL_HOME", "")
	t.Setenv("OBEY_INSTALLER_HOME", target)

	if err := EnsureHome(context.Background(), 0700); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("want mode 0700, got %o", info.Mode().Perm())
	}
	if err := EnsureHome(context.Background(), 0700); err != nil {
		t.Fatalf("second EnsureHome: %v", err)
	}
}
