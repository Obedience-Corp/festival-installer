package app

// FestivalPackageID is the marketplace ID for the camp+fest suite bundle.
const FestivalPackageID = "obedience-corp/festival"

// InstallResult is returned by install operations.
type InstallResult struct {
	Package       string        `json:"package"`
	Version       string        `json:"version"`
	Channel       string        `json:"channel"`
	Source        string        `json:"source"`
	Files         []string      `json:"files"`
	SelfPlacement SelfPlacement `json:"self_placement,omitempty"`
	SelfPath      string        `json:"self_path,omitempty"`
	// SelfSkipped is true when the manifest named the hub itself but it was
	// left in place because the running hub is not the managed binary.
	SelfSkipped bool `json:"self_skipped,omitempty"`
}

// UpdateResult is returned by update operations.
type UpdateResult struct {
	Package       string        `json:"package"`
	Action        string        `json:"action"` // upgraded | current | unmanaged | absent | package
	Version       string        `json:"version,omitempty"`
	Latest        string        `json:"latest,omitempty"`
	Upgrade       string        `json:"upgrade,omitempty"`
	From          string        `json:"from,omitempty"`
	SelfPlacement SelfPlacement `json:"self_placement,omitempty"`
	SelfPath      string        `json:"self_path,omitempty"`
	// SelfReplaced is true when this update replaced the running hub binary,
	// which means the process printing this message is the previous version.
	SelfReplaced bool `json:"self_replaced"`
}

// UninstallResult is returned by uninstall operations.
type UninstallResult struct {
	Package string   `json:"package"`
	Removed []string `json:"removed,omitempty"`
	Note    string   `json:"note,omitempty"`
}

// BrowseEntry is one package in a browse group.
type BrowseEntry struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"display_name"`
	Class        string   `json:"class"`
	Summary      string   `json:"summary,omitempty"`
	HostRuntimes []string `json:"host_runtimes"`
	Channels     []string `json:"channels"`
	Source       string   `json:"source"`
	Verified     bool     `json:"verified"`
}

// BrowseGroup groups packages by host runtime.
type BrowseGroup struct {
	HostRuntime string        `json:"host_runtime"`
	Packages    []BrowseEntry `json:"packages"`
}

// BrowseResult is the browse catalog view model.
type BrowseResult struct {
	Groups []BrowseGroup `json:"groups"`
}

// ListEntry is one installed package.
type ListEntry struct {
	PackageID   string   `json:"package_id"`
	Version     string   `json:"version"`
	Channel     string   `json:"channel"`
	Source      string   `json:"source"`
	Origin      string   `json:"origin,omitempty"` // package | leftover | empty for receipts
	InstalledAt string   `json:"installed_at"`
	Files       []string `json:"files"`
}

// ListResult is the installed inventory.
type ListResult struct {
	Packages []ListEntry `json:"packages"`
}

// Doctor check statuses. Pending is distinct from Warn on purpose: "you have
// not finished setting up" is not "here is a mild concern about a working
// install", and only Fail decides the exit code.
const (
	DoctorOK      = "ok"
	DoctorWarn    = "warn"
	DoctorPending = "pending"
	DoctorFail    = "fail"
)

// DoctorCheck is one health check result.
type DoctorCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"` // ok | warn | pending | fail
	Message string `json:"message"`
}

// DoctorData wraps doctor checks for JSON.
type DoctorData struct {
	Checks []DoctorCheck `json:"checks"`
}

// WhichResult resolves where a tool binary lives.
type WhichResult struct {
	Tool     string         `json:"tool"`
	Path     string         `json:"path,omitempty"`
	Managed  string         `json:"managed,omitempty"`
	OnPath   bool           `json:"on_path"`
	Shadowed bool           `json:"shadowed"`
	Origin   OriginKind     `json:"origin,omitempty"`
	Flavor   PackageFlavor  `json:"flavor,omitempty"`
	All      []ToolLocation `json:"all,omitempty"`
}

// StatusSummary powers the TUI home status strip.
type StatusSummary struct {
	Installed        bool           `json:"installed"`
	Version          string         `json:"version,omitempty"`
	Channel          string         `json:"channel,omitempty"`
	Source           string         `json:"source,omitempty"`
	ManagedBin       string         `json:"managed_bin"`
	ManagedBinOnPath bool           `json:"managed_bin_on_path"`
	Action           string         `json:"action,omitempty"` // absent | managed | unmanaged | package
	Origin           OriginKind     `json:"origin,omitempty"`
	Flavor           PackageFlavor  `json:"flavor,omitempty"`
	Package          string         `json:"package,omitempty"`
	Prefix           string         `json:"prefix,omitempty"`
	Helper           string         `json:"helper,omitempty"`
	Upgrade          string         `json:"upgrade,omitempty"`
	Remove           string         `json:"remove,omitempty"`
	Dual             bool           `json:"dual,omitempty"`
	ShadowNote       string         `json:"shadow_note,omitempty"`
	Shadows          []ToolLocation `json:"shadows,omitempty"`
	// Setup is the one authoritative answer to "how far along is this home",
	// shared with doctor and the CLI. ManagedBin and ManagedBinOnPath above
	// repeat two of its fields so existing TUI renders keep compiling; Setup is
	// the field new code should read.
	Setup SetupState `json:"setup"`
	// Latest is channel-latest from a TUI/update probe; Status() never sets it.
	Latest string `json:"latest,omitempty"`
}
