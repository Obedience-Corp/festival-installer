package tui

import "github.com/Obedience-Corp/obey-installer/internal/launch"

// SessionResult is returned when one hub TUI program ends.
// The outer festival loop either quits or runs Launch then restarts the hub.
type SessionResult struct {
	// Quit means the user left the hub entirely.
	Quit bool
	// Launch is set when the user selected a launchpad tool; hub TUI has exited
	// alt-screen so the parent can run the child on the real TTY.
	Launch *launch.Spec
	// Err is a fatal hub error (not a child exit code).
	Err error
	// Banner is an optional soft message to show after returning from a child.
	Banner string
}
