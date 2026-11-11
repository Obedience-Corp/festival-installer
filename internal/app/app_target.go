package app

// IsAppTarget reports whether target names Festival App on the command line.
func IsAppTarget(target string) bool {
	switch target {
	case "app", "festival-app", AppPackageID:
		return true
	}
	return false
}
