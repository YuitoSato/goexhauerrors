package internal

import (
	"strings"
	"sync"
)

var (
	ignorePackages string
	mu             sync.RWMutex
)

// SetIgnorePackages sets the comma-separated list of package paths to ignore.
func SetIgnorePackages(s string) {
	mu.Lock()
	ignorePackages = s
	mu.Unlock()
}

// ShouldIgnorePackage checks if a package path is in the ignore list.
func ShouldIgnorePackage(pkgPath string) bool {
	mu.RLock()
	pkgs := ignorePackages
	mu.RUnlock()

	if pkgs == "" {
		return false
	}
	for _, ignored := range strings.Split(pkgs, ",") {
		if strings.TrimSpace(ignored) == pkgPath {
			return true
		}
	}
	return false
}
