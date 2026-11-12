//go:build !unix

package app

import "io/fs"

// ownedByUID has no meaning off Unix; the deb path never runs there.
func ownedByUID(fs.FileInfo, int) bool { return true }
