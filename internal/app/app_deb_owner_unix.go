//go:build unix

package app

import (
	"io/fs"
	"syscall"
)

// ownedByUID reports whether info's owner is uid.
func ownedByUID(info fs.FileInfo, uid int) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == uid
}
