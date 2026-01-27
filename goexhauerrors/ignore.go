package goexhauerrors

import "strings"

// shouldIgnorePackage checks if a package path is in the ignore list.
func shouldIgnorePackage(pkgPath string) bool {
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
