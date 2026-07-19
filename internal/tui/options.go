package tui

import "os"

// Options configure the manager TUI.
type Options struct {
	Version string
	// SkipBoot skips the splash (used after returning from a child tool).
	SkipBoot bool
	// Stderr overrides stderr for launch banners; nil uses os.Stderr.
	Stderr *os.File
}

func (o Options) stderr() *os.File {
	if o.Stderr != nil {
		return o.Stderr
	}
	return os.Stderr
}
