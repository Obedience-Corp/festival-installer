package launch

// Spec describes a tool invocation the hub should run after leaving the TUI.
type Spec struct {
	// Tool is the binary name: camp, fest, or a plugin executable name.
	Tool string
	// Args are passed to the tool (e.g. {"wi"}, {"watch"}).
	Args []string
	// Dir is the working directory; empty means inherit the hub process cwd.
	Dir string
	// Title is a short human label for status banners (optional).
	Title string
}

// Result is the outcome of a child process run.
type Result struct {
	ExitCode int
	Err      error
	// Path is the resolved binary that was executed (empty if resolve failed).
	Path string
}
