package hosts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/festival-installer/internal/metadata"
	"github.com/Obedience-Corp/festival-installer/internal/state"
	"github.com/Obedience-Corp/festival-installer/internal/state/receipts"
)

type fakeRunner struct {
	lookErr error
	path    string
	out     []byte
	outErr  error
}

func (f fakeRunner) LookPath(string) (string, error) {
	if f.lookErr != nil {
		return "", f.lookErr
	}
	return f.path, nil
}

func (f fakeRunner) Output(context.Context, string, ...string) ([]byte, error) {
	return f.out, f.outErr
}

func campWith(r Runner) *Adapter { return &Adapter{runner: r, bin: "camp"} }

func TestDetectInstalled_NotOnPath(t *testing.T) {
	a := campWith(fakeRunner{lookErr: errors.New("not found")})
	installed, version, _, err := a.DetectInstalled(context.Background())
	if err != nil {
		t.Fatalf("DetectInstalled: %v", err)
	}
	if installed || version != "" {
		t.Fatalf("expected not installed, got installed=%v version=%q", installed, version)
	}
}

func TestDetectInstalled_PresentWithVersion(t *testing.T) {
	a := campWith(fakeRunner{path: "/usr/local/bin/camp", out: []byte("camp 0.2.11\ncommit: abc\n")})
	installed, version, path, err := a.DetectInstalled(context.Background())
	if err != nil {
		t.Fatalf("DetectInstalled: %v", err)
	}
	if !installed || version != "0.2.11" || path != "/usr/local/bin/camp" {
		t.Fatalf("unexpected: installed=%v version=%q path=%q", installed, version, path)
	}
}

func TestDetectInstalled_PresentVersionErrors(t *testing.T) {
	a := campWith(fakeRunner{path: "/usr/local/bin/camp", outErr: errors.New("boom")})
	installed, version, _, err := a.DetectInstalled(context.Background())
	if err != nil {
		t.Fatalf("DetectInstalled: %v", err)
	}
	if !installed || version != "" {
		t.Fatalf("expected installed with empty version, got installed=%v version=%q", installed, version)
	}
}

func TestRemove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("OBEY_INSTALLER_HOME", home)
	ctx := context.Background()

	binDir, err := state.BinDir(ctx)
	if err != nil {
		t.Fatalf("BinDir: %v", err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir binDir: %v", err)
	}
	dst := filepath.Join(binDir, "fest-demo")
	if err := os.WriteFile(dst, []byte("fest-demo-bytes"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	a := NewAdapter("fest")
	if err := a.Remove(ctx, receipts.Receipt{OwnedFiles: []receipts.OwnedFile{{Path: dst}}}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Fatalf("expected removal, stat err=%v", statErr)
	}
}

func TestValidateCompatibility(t *testing.T) {
	cases := []struct {
		name    string
		runner  fakeRunner
		target  metadata.RuntimeTarget
		wantErr string
	}{
		{"host mismatch", fakeRunner{path: "/b/camp", out: []byte("camp 0.2.11")}, metadata.RuntimeTarget{Runtime: "fest"}, "E_HOST_MISMATCH"},
		{"satisfied", fakeRunner{path: "/b/camp", out: []byte("camp 0.4.5")}, metadata.RuntimeTarget{Runtime: "camp-cli", VersionConstraint: ">=0.4.0"}, ""},
		{"unsatisfied", fakeRunner{path: "/b/camp", out: []byte("camp 0.3.0")}, metadata.RuntimeTarget{Runtime: "camp-cli", VersionConstraint: ">=0.4.0"}, "E_HOST_INCOMPATIBLE"},
		{"no constraint", fakeRunner{path: "/b/camp", out: []byte("camp 0.2.11")}, metadata.RuntimeTarget{Runtime: "camp"}, ""},
		{"host absent", fakeRunner{lookErr: errors.New("nope")}, metadata.RuntimeTarget{Runtime: "camp", VersionConstraint: ">=0.4.0"}, "E_HOST_ABSENT"},
		{"dev host allows constraint", fakeRunner{path: "/b/camp", out: []byte("camp dev\ncommit: abc")}, metadata.RuntimeTarget{Runtime: "camp-cli", VersionConstraint: ">=0.4.0"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := campWith(tc.runner)
			err := a.ValidateCompatibility(context.Background(), tc.target)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %s, got %v", tc.wantErr, err)
			}
		})
	}
}
