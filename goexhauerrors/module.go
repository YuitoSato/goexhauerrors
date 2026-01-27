package goexhauerrors

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/analysis"
)

var (
	modulePathCache = make(map[string]string)
	modulePathMutex sync.RWMutex
)

// getModulePath returns the module path for the given package.
// It reads go.mod file to determine the module path.
func getModulePath(pass *analysis.Pass) string {
	pkgPath := pass.Pkg.Path()

	modulePathMutex.RLock()
	if cached, ok := modulePathCache[pkgPath]; ok {
		modulePathMutex.RUnlock()
		return cached
	}
	modulePathMutex.RUnlock()

	modulePath := findModulePath(pass)

	modulePathMutex.Lock()
	modulePathCache[pkgPath] = modulePath
	modulePathMutex.Unlock()

	return modulePath
}

// findModulePath searches for go.mod file starting from the package directory.
func findModulePath(pass *analysis.Pass) string {
	// Get directory from the first file in the package
	for _, file := range pass.Files {
		pos := pass.Fset.Position(file.Pos())
		if pos.Filename != "" {
			dir := filepath.Dir(pos.Filename)
			return findGoModPath(dir)
		}
	}
	return ""
}

// findGoModPath searches for go.mod file by walking up the directory tree.
func findGoModPath(dir string) string {
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(goModPath); err == nil {
			if modFile, err := modfile.Parse(goModPath, data, nil); err == nil {
				return modFile.Module.Mod.Path
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// isModulePackage checks if the given package path belongs to the current module.
func isModulePackage(modulePath, pkgPath string) bool {
	// First, check if it looks like an external package (has domain name)
	// This handles both test packages and real external packages
	if looksLikeExternalPackage(pkgPath) {
		// It looks like an external package, but check if it's actually part of the module
		if modulePath != "" && (pkgPath == modulePath || strings.HasPrefix(pkgPath, modulePath+"/")) {
			return true
		}
		return false
	}

	// Doesn't look like an external package (no domain name)
	// This is either a test package or a local package in a simple setup
	if modulePath == "" {
		// No go.mod found, treat as local
		return true
	}

	// Has module path, check if it matches
	if pkgPath == modulePath || strings.HasPrefix(pkgPath, modulePath+"/") {
		return true
	}

	// Package doesn't match module path, but also doesn't look external
	// This happens in tests where packages have simple names like "basic"
	// Treat as local for backwards compatibility
	return true
}

// looksLikeExternalPackage checks if a package path looks like an external package.
// External packages typically have domain names in their first path component.
func looksLikeExternalPackage(pkgPath string) bool {
	// Check if it's a standard library package
	if isStandardLibrary(pkgPath) {
		return true
	}

	// External packages have domain names (contain ".") in the first component
	// e.g., "github.com/user/repo", "gorm.io/gorm", "golang.org/x/tools"
	parts := strings.Split(pkgPath, "/")
	if len(parts) > 0 && strings.Contains(parts[0], ".") {
		return true
	}

	return false
}

// isStandardLibrary checks if a package path is a Go standard library package.
func isStandardLibrary(pkgPath string) bool {
	// Common standard library root packages
	stdlibRoots := map[string]bool{
		"archive": true, "bufio": true, "bytes": true, "compress": true,
		"container": true, "context": true, "crypto": true, "database": true,
		"debug": true, "embed": true, "encoding": true, "errors": true,
		"expvar": true, "flag": true, "fmt": true, "go": true,
		"hash": true, "html": true, "image": true, "index": true,
		"io": true, "log": true, "maps": true, "math": true,
		"mime": true, "net": true, "os": true, "path": true,
		"plugin": true, "reflect": true, "regexp": true, "runtime": true,
		"slices": true, "sort": true, "strconv": true, "strings": true,
		"sync": true, "syscall": true, "testing": true, "text": true,
		"time": true, "unicode": true, "unsafe": true,
	}

	parts := strings.Split(pkgPath, "/")
	if len(parts) > 0 {
		return stdlibRoots[parts[0]]
	}
	return false
}
