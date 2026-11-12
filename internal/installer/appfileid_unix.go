//go:build unix

package installer

import (
	"os"
	"syscall"

	errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"
)

// fileID returns the device and inode of path without following symlinks.
func fileID(path string) (dev, ino uint64, err error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, errpkg.New("E_INSTALL_RECONCILE", "no file identity for "+path)
	}
	return uint64(st.Dev), uint64(st.Ino), nil
}
