package internal

import "strings"

var ignorePackages string

// SetIgnorePackages sets the comma-separated list of package paths to ignore.
func SetIgnorePackages(s string) {
	ignorePackages = s
}

// ShouldIgnorePackage checks if a package path is in the ignore list.
func ShouldIgnorePackage(pkgPath string) bool {
	if ignorePackages == "" {
		return false
	}
	for _, ignored := range strings.Split(ignorePackages, ",") {
		if strings.TrimSpace(ignored) == pkgPath {
			return true
		}
	}
	return false
}
