package app

import (
	"context"
	"os"
	"path/filepath"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
	"github.com/Obedience-Corp/festival-installer/internal/state"
)

// SelfPlacement describes where the running hub binary lives relative to the
// managed bin dir. It is deliberately distinct from the package-level
// "unmanaged" status (see Status and handleUnmanaged), which describes camp
// and fest receipts rather than the hub's own executable.
type SelfPlacement string

const (
	SelfManaged  SelfPlacement = "managed"
	SelfExternal SelfPlacement = "external"
	SelfUnknown  SelfPlacement = "unknown"
)

// ResolveSelf reports whether the running executable is the hub binary
// managed by this installation. It resolves symlinks on both sides so a
// symlinked managed bin dir, a relative argv[0], and a PATH lookup all agree.
func ResolveSelf(ctx context.Context) (SelfPlacement, string, error) {
	if err := ctx.Err(); err != nil {
		return SelfUnknown, "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return SelfUnknown, "", errpkg.Wrap("E_SELF_EXECUTABLE", err, "resolve running executable")
	}
	binDir, err := state.BinDir(ctx)
	if err != nil {
		return SelfUnknown, exe, err
	}
	managed := filepath.Join(binDir, selfBinaryName)
	if samePath(exe, managed) {
		return SelfManaged, managed, nil
	}
	return SelfExternal, exe, nil
}
