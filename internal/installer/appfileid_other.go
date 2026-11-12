//go:build !unix

package installer

import errpkg "github.com/Obedience-Corp/festival-installer/internal/errors"

func fileID(string) (dev, ino uint64, err error) {
	return 0, 0, errpkg.New("E_INSTALL_RECONCILE", "beside-target swaps are not supported on this platform")
}
