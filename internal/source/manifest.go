package source

type Marketplace struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	Homepage      string         `json:"homepage,omitempty"`
	SchemaVersion string         `json:"schema_version"`
	Namespace     string         `json:"namespace,omitempty"`
	Packages      []PackageRef   `json:"packages"`
	Extensions    map[string]any `json:"extensions,omitempty"`
}

type PackageRef struct {
	ID           string   `json:"id"`
	DisplayName  string   `json:"display_name"`
	Class        string   `json:"class"`
	Summary      string   `json:"summary,omitempty"`
	HostRuntimes []string `json:"host_runtimes,omitempty"`
	Channels     []string `json:"channels,omitempty"`
	ManifestPath string   `json:"manifest_path"`
}
