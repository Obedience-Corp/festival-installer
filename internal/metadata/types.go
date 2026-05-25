package metadata

import "time"

// Source describes a marketplace source: an identifier, the URL where its
// index lives, and the set of pinned verification keys.
type Source struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	IndexURL   string                  `json:"indexUrl"`
	Keys       map[string]SourceKey    `json:"keys"`
	TTLSeconds int                     `json:"ttlSeconds"`
	Extensions map[string]any          `json:"extensions,omitempty"`
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
	ID       string   `json:"id"`
	Channels []string `json:"channels"`
}

// PackageManifest describes a single installable package version.
type PackageManifest struct {
	ID         string         `json:"id"`
	Version    string         `json:"version"`
	Channel    string         `json:"channel"`
	Artifacts  []Artifact     `json:"artifacts"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

type Artifact struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
	Sha256   string `json:"sha256"`
}
