package metadata

// Festival App artifact kinds. Only the App's own manifest uses them (D003);
// the suite manifest never does, so released installers never load them.
const (
	ArtifactKindAppBundle = "app-bundle"
	ArtifactKindNSIS      = "nsis"
	ArtifactKindDeb       = "deb"
	ArtifactKindAppImage  = "appimage"

	InstallKindDesktopApp = "desktop_app"
)

// IsAppArtifactKind reports whether kind is one of the Festival App kinds.
func IsAppArtifactKind(kind string) bool {
	switch kind {
	case ArtifactKindAppBundle, ArtifactKindNSIS, ArtifactKindDeb, ArtifactKindAppImage:
		return true
	}
	return false
}
