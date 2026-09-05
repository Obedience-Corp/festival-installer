package tui

import (
	"github.com/Obedience-Corp/festival-installer/internal/app"
	"github.com/Obedience-Corp/festival-installer/internal/launch"
)

// SessionResult is returned when one hub TUI program ends.
// The outer festival loop either quits or runs Launch then restarts the hub.
type SessionResult struct {
	// Quit means the user left the hub entirely.
	Quit bool
	// Launch is set when the user selected a launchpad tool; hub TUI has exited
	// alt-screen so the parent can run the child on the real TTY.
	Launch *launch.Spec
	// ResumeCursor is the cursor position to restore after the child exits.
	ResumeCursor int
	// ResumeScreen is the screen the launch was started from, so the hub comes
	// back where the user left it rather than always on the launchpad.
	ResumeScreen screen
	// RecordTourStep names a getting-started tour step whose completion should
	// be recorded if the child exits cleanly. Empty for ordinary launches.
	RecordTourStep app.TourStepKey
	// Err is a fatal hub error (not a child exit code).
	Err error
	// Banner is an optional soft message to show after returning from a child.
	Banner string
}
