package goexhauerrors

import (
	"strings"
	"sync"
)

var (
	ignoredPackagesMu       sync.Mutex
	parsedIgnorePackages    string
	parsedIgnorePackagesSet map[string]bool
)

// shouldIgnorePackage checks if a package path is in the ignore list.
func shouldIgnorePackage(pkgPath string) bool {
	ignoredPackagesMu.Lock()
	if ignorePackages != parsedIgnorePackages || parsedIgnorePackagesSet == nil {
		parsedIgnorePackages = ignorePackages
		parsedIgnorePackagesSet = make(map[string]bool)
		if ignorePackages != "" {
			for _, pkg := range strings.Split(ignorePackages, ",") {
				trimmed := strings.TrimSpace(pkg)
				if trimmed != "" {
					parsedIgnorePackagesSet[trimmed] = true
				}
			}
		}
	}
	set := parsedIgnorePackagesSet
	ignoredPackagesMu.Unlock()
	return set[pkgPath]
}
