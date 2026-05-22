package receipts

import (
	"os"
	"time"
)

type Receipt struct {
	PackageID   string
	Version     string
	Source      string
	Channel     string
	InstalledAt time.Time
	OwnedFiles  []OwnedFile
	Metadata    map[string]string
}

type OwnedFile struct {
	Path string
	Hash string
	Mode os.FileMode
}

type Filter struct {
	Source         string
	Version        string
	InstalledAfter time.Time
}
