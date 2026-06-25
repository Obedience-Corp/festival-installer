package metadata

import "time"

// Source describes a marketplace source: an identifier, the URL where its
// index lives, and the set of pinned verification keys.
type Source struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	IndexURL   string               `json:"indexUrl"`
	Keys       map[string]SourceKey `json:"keys"`
	TTLSeconds int                  `json:"ttlSeconds"`
	Extensions map[string]any       `json:"extensions,omitempty"`
}

type SourceKey struct {
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"publicKey"` // base64-std
}

// Index lists the packages a Source publishes.
type Index struct {
	Source     string         `json:"source"`
	UpdatedAt  time.Time      `json:"updatedAt"`
	Packages   []IndexEntry   `json:"packages"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

type IndexEntry struct {
	ID           string          `json:"id"`
	Channels     []string        `json:"channels"`
	Class        string          `json:"class,omitempty"`
	DisplayName  string          `json:"display_name,omitempty"`
	Summary      string          `json:"summary,omitempty"`
	HostRuntimes []string        `json:"host_runtimes,omitempty"`
	Targets      []RuntimeTarget `json:"targets,omitempty"`
}

// PackageManifest describes an installable package and its published releases.
type PackageManifest struct {
	SchemaVersion    int             `json:"schema_version"`
	ID               string          `json:"id"`
	Class            string          `json:"class"`
	DisplayName      string          `json:"display_name"`
	Summary          string          `json:"summary"`
	Description      string          `json:"description,omitempty"`
	Homepage         string          `json:"homepage,omitempty"`
	Licenses         []string        `json:"licenses,omitempty"`
	Aliases          []string        `json:"aliases,omitempty"`
	Tags             []string        `json:"tags,omitempty"`
	SupportedScopes  []string        `json:"supported_scopes,omitempty"`
	ProvidesBinaries []string        `json:"provides_binaries,omitempty"`
	HostRuntimes     []HostRuntime   `json:"host_runtimes,omitempty"`
	Targets          []RuntimeTarget `json:"targets,omitempty"`
	Releases         []Release       `json:"releases"`
	Extensions       map[string]any  `json:"extensions,omitempty"`
}

type HostRuntime struct {
	Runtime     string   `json:"runtime"`
	DisplayName string   `json:"display_name,omitempty"`
	Features    []string `json:"features,omitempty"`
}

type RuntimeTarget struct {
	Package           string   `json:"package"`
	Runtime           string   `json:"runtime"`
	VersionConstraint string   `json:"version_constraint,omitempty"`
	RequiredFeatures  []string `json:"required_features,omitempty"`
}

type Release struct {
	Version          string            `json:"version"`
	Channel          string            `json:"channel"`
	PublishedAt      time.Time         `json:"published_at"`
	Components       map[string]string `json:"components,omitempty"`
	ProvidesBinaries []string          `json:"provides_binaries,omitempty"`
	Compatibility    Compatibility     `json:"compatibility"`
	Dependencies     []Dependency      `json:"dependencies"`
	Artifacts        []Artifact        `json:"artifacts"`
	Install          Install           `json:"install"`
}

type Compatibility struct {
	OS   []string `json:"os"`
	Arch []string `json:"arch"`
}

type Dependency struct {
	Package           string `json:"package"`
	VersionConstraint string `json:"version_constraint"`
	Scope             string `json:"scope,omitempty"`
}

type Artifact struct {
	Kind     string   `json:"kind"`
	OS       string   `json:"os"`
	Arch     string   `json:"arch"`
	URL      string   `json:"url"`
	Sha256   string   `json:"sha256"`
	Filename string   `json:"filename,omitempty"`
	Binaries []string `json:"binaries,omitempty"`
}

type Install struct {
	Entries []InstallEntry `json:"entries"`
}

type InstallEntry struct {
	Kind           string `json:"kind"`
	Source         string `json:"source"`
	ExecutableName string `json:"executable_name,omitempty"`
	ExtensionName  string `json:"extension_name,omitempty"`
	SkillSlug      string `json:"skill_slug,omitempty"`
}
