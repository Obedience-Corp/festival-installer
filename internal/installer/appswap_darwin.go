//go:build darwin

package installer

import "golang.org/x/sys/unix"

// exchange atomically swaps two paths. Volumes without RENAME_SWAP report
// errExchangeUnsupported so the caller can use two renames.
func exchange(a, b string) error {
	err := unix.RenamexNp(a, b, unix.RENAME_SWAP)
	if err == unix.ENOTSUP || err == unix.EINVAL {
		return errExchangeUnsupported
	}
	return err
}
