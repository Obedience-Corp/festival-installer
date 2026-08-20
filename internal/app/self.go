package app

import (
	"context"
	"errors"
	"io/fs"
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
//
// Placement is a report, not a failure: a resolution error on either path
// (other than the managed path simply not existing) yields SelfUnknown with a
// nil error rather than propagating, since destructive callers must fail
// closed on SelfUnknown themselves instead of tripping over a transient stat
// error here.
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

	resolvedExe, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return SelfUnknown, exe, nil
	}
	resolvedManaged, err := filepath.EvalSymlinks(managed)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return SelfExternal, exe, nil
		}
		return SelfUnknown, exe, nil
	}
	if resolvedExe == resolvedManaged {
		return SelfManaged, managed, nil
	}
	return SelfExternal, exe, nil
}

// SelfSkippedNote is the human-readable explanation for why the hub binary
// was left untouched by an install or update, phrased according to whether
// placement was confirmed external or could not be resolved at all. Install
// and update share this wording so both commands describe the same skip the
// same way.
func SelfSkippedNote(placement SelfPlacement, selfPath string) string {
	if placement == SelfUnknown {
		return "could not determine whether the running festival binary is managed; festival binary left untouched"
	}
	return "festival is installed outside this managed bin dir (" + selfPath + "); left untouched"
}
