package launch

// Mode selects how the hub runs a launchpad entry. The zero value is ModeTUI.
type Mode string

const (
	// ModeTUI hands the child the real TTY: suspend hub, exec, resume.
	ModeTUI Mode = ""
	// ModeOneShot captures a short-lived command's output into the hub pager
	// instead of losing it when the hub repaints.
	ModeOneShot Mode = "oneshot"
	// ModeStream pipes a long-running command's output live into the hub pager.
	ModeStream Mode = "stream"
)

// IsCapture reports whether the entry runs inside the hub with piped output
// (one-shot and streaming modes) rather than owning the TTY.
func (m Mode) IsCapture() bool { return m == ModeOneShot || m == ModeStream }

// Spec describes a tool invocation the hub should run after leaving the TUI.
type Spec struct {
	// Tool is the binary name: camp, fest, or a plugin executable name.
	Tool string
	// Args are passed to the tool (e.g. {"wi"}, {"watch"}).
	Args []string
	// Dir is the working directory; empty means inherit the hub process cwd
	// (or campaign root when DetectCampaignRoot finds one for campaign tools).
	Dir string
	// Title is a short human label for status banners (optional).
	Title string
}

// Result is the outcome of a child process run.
type Result struct {
	// ExitCode is 0 on success, the process exit status on normal exit,
	// 128+signum when terminated by signal (Unix), or -1 when the process
	// never started / resolve failed.
	ExitCode int
	// Err is set on non-zero exit, signal death, or start failure.
	Err error
	// Path is the resolved binary that was executed (empty if resolve failed).
	Path string
	// Started is true when the child process was actually spawned.
	// Distinguishes signal/exit failures from resolve/start failures.
	Started bool
	// Signal is the signal name when the child was terminated by signal
	// (e.g. "interrupt"); empty otherwise.
	Signal string
}
