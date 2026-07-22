package hosts

import (
	"context"
	"os/exec"
	"strings"

	errpkg "github.com/Obedience-Corp/obey-installer/internal/errors"
	"github.com/Obedience-Corp/obey-installer/internal/hosts/shared"
	"github.com/Obedience-Corp/obey-installer/internal/installer"
	"github.com/Obedience-Corp/obey-installer/internal/metadata"
	"github.com/Obedience-Corp/obey-installer/internal/state/receipts"
)

type Host interface {
	HostID() string
	DetectInstalled(ctx context.Context) (bool, string, string, error)
	ValidateCompatibility(ctx context.Context, target metadata.RuntimeTarget) error
	Remove(ctx context.Context, r receipts.Receipt) error
}

type Runner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	LookPath(name string) (string, error)
}

type execRunner struct{}

func (execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func (execRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }

type Adapter struct {
	runner Runner
	bin    string
}

func NewAdapter(bin string) *Adapter { return &Adapter{runner: execRunner{}, bin: bin} }

func (a *Adapter) HostID() string { return a.bin }

func (a *Adapter) DetectInstalled(ctx context.Context) (bool, string, string, error) {
	if err := ctx.Err(); err != nil {
		return false, "", "", errpkg.Wrap("E_HOST_CTX", err, "context cancelled")
	}
	path, err := a.runner.LookPath(a.bin)
	if err != nil {
		return false, "", "", nil
	}
	out, err := a.runner.Output(ctx, a.bin, "version")
	if err != nil {
		return true, "", path, nil
	}
	return true, parseVersion(string(out)), path, nil
}

func (a *Adapter) ValidateCompatibility(ctx context.Context, target metadata.RuntimeTarget) error {
	if target.Runtime != "" && target.Runtime != a.bin && !strings.HasPrefix(target.Runtime, a.bin+"-") {
		return errpkg.New("E_HOST_MISMATCH", "plugin targets runtime "+target.Runtime+", not "+a.bin)
	}
	installed, version, _, err := a.DetectInstalled(ctx)
	if err != nil {
		return err
	}
	if !installed {
		return errpkg.New("E_HOST_ABSENT", a.bin+" is not installed")
	}
	if target.VersionConstraint == "" || !isSemver(version) {
		return nil
	}
	ok, cerr := satisfies(version, target.VersionConstraint)
	if cerr != nil {
		return cerr
	}
	if !ok {
		return errpkg.New("E_HOST_INCOMPATIBLE", a.bin+" "+version+" does not satisfy "+target.VersionConstraint)
	}
	return nil
}

func (a *Adapter) Remove(ctx context.Context, r receipts.Receipt) error {
	for _, f := range r.OwnedFiles {
		if err := shared.RemoveByRecord(ctx, f); err != nil {
			return err
		}
	}
	return nil
}

func parseVersion(out string) string {
	for f := range strings.FieldsSeq(out) {
		f = strings.TrimPrefix(f, "v")
		if isSemver(f) {
			return f
		}
	}
	return ""
}

func isSemver(v string) bool {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 3 {
		return false
	}
	if !allDigits(parts[0]) || !allDigits(parts[1]) {
		return false
	}
	return len(parts[2]) > 0 && parts[2][0] >= '0' && parts[2][0] <= '9'
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func satisfies(version, constraint string) (bool, error) {
	op, want := splitConstraint(strings.TrimSpace(constraint))
	if want == "" {
		return false, errpkg.New("E_HOST_CONSTRAINT", "no version in constraint "+constraint)
	}
	switch op {
	case ">=":
		return !installer.VersionLess(version, want), nil
	case ">":
		return installer.VersionLess(want, version), nil
	case "<=":
		return !installer.VersionLess(want, version), nil
	case "<":
		return installer.VersionLess(version, want), nil
	case "=", "":
		return !installer.VersionLess(version, want) && !installer.VersionLess(want, version), nil
	default:
		return false, errpkg.New("E_HOST_CONSTRAINT", "unsupported constraint "+constraint)
	}
}

func splitConstraint(c string) (string, string) {
	for _, op := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(c, op) {
			return op, strings.TrimSpace(c[len(op):])
		}
	}
	return "", c
}
