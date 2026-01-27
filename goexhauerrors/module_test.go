package goexhauerrors

import "testing"

func TestIsModulePackage(t *testing.T) {
	tests := []struct {
		name       string
		modulePath string
		pkgPath    string
		want       bool
	}{
		// Module path is set (real project with go.mod)
		{
			name:       "same module - exact match",
			modulePath: "github.com/myorg/myproject",
			pkgPath:    "github.com/myorg/myproject",
			want:       true,
		},
		{
			name:       "same module - subpackage",
			modulePath: "github.com/myorg/myproject",
			pkgPath:    "github.com/myorg/myproject/pkg/errors",
			want:       true,
		},
		{
			name:       "external package - github",
			modulePath: "github.com/myorg/myproject",
			pkgPath:    "github.com/kelseyhightower/envconfig",
			want:       false,
		},
		{
			name:       "external package - gorm",
			modulePath: "github.com/myorg/myproject",
			pkgPath:    "gorm.io/gorm",
			want:       false,
		},
		{
			name:       "stdlib - errors",
			modulePath: "github.com/myorg/myproject",
			pkgPath:    "errors",
			want:       false,
		},
		{
			name:       "stdlib - strconv",
			modulePath: "github.com/myorg/myproject",
			pkgPath:    "strconv",
			want:       false,
		},
		{
			name:       "stdlib - database/sql",
			modulePath: "github.com/myorg/myproject",
			pkgPath:    "database/sql",
			want:       false,
		},

		// Module path is empty (test environment without go.mod)
		{
			name:       "no module - simple package name",
			modulePath: "",
			pkgPath:    "basic",
			want:       true,
		},
		{
			name:       "no module - nested test package",
			modulePath: "",
			pkgPath:    "crosspkg/errors",
			want:       true,
		},
		{
			name:       "no module - external with domain",
			modulePath: "",
			pkgPath:    "github.com/someorg/somepkg",
			want:       false,
		},
		{
			name:       "no module - stdlib",
			modulePath: "",
			pkgPath:    "errors",
			want:       false,
		},
		{
			name:       "no module - stdlib nested",
			modulePath: "",
			pkgPath:    "encoding/json",
			want:       false,
		},

		// Edge cases
		{
			name:       "test package with module path found",
			modulePath: "github.com/YuitoSato/goexhauerrors",
			pkgPath:    "basic",
			want:       true, // Simple name without domain should be treated as local
		},
		{
			name:       "partial match should not match",
			modulePath: "github.com/myorg/myproject",
			pkgPath:    "github.com/myorg/myprojectfoo",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isModulePackage(tt.modulePath, tt.pkgPath)
			if got != tt.want {
				t.Errorf("isModulePackage(%q, %q) = %v, want %v", tt.modulePath, tt.pkgPath, got, tt.want)
			}
		})
	}
}

func TestLooksLikeExternalPackage(t *testing.T) {
	tests := []struct {
		name    string
		pkgPath string
		want    bool
	}{
		// External packages (should return true)
		{"github.com package", "github.com/user/repo", true},
		{"gorm.io package", "gorm.io/gorm", true},
		{"golang.org package", "golang.org/x/tools", true},
		{"stdlib errors", "errors", true},
		{"stdlib strconv", "strconv", true},
		{"stdlib fmt", "fmt", true},
		{"stdlib database/sql", "database/sql", true},
		{"stdlib encoding/json", "encoding/json", true},

		// Local/test packages (should return false)
		{"simple test package", "basic", false},
		{"nested test package", "crosspkg/errors", false},
		{"another test package", "mypackage", false},
		{"test subpackage", "mypackage/subpkg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeExternalPackage(tt.pkgPath)
			if got != tt.want {
				t.Errorf("looksLikeExternalPackage(%q) = %v, want %v", tt.pkgPath, got, tt.want)
			}
		})
	}
}

func TestIsStandardLibrary(t *testing.T) {
	tests := []struct {
		name    string
		pkgPath string
		want    bool
	}{
		// Standard library packages
		{"errors", "errors", true},
		{"fmt", "fmt", true},
		{"io", "io", true},
		{"os", "os", true},
		{"strconv", "strconv", true},
		{"database/sql", "database/sql", true},
		{"encoding/json", "encoding/json", true},
		{"net/http", "net/http", true},
		{"context", "context", true},

		// Not standard library
		{"github package", "github.com/user/repo", false},
		{"gorm package", "gorm.io/gorm", false},
		{"simple name", "mypackage", false},
		{"nested name", "crosspkg/errors", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStandardLibrary(tt.pkgPath)
			if got != tt.want {
				t.Errorf("isStandardLibrary(%q) = %v, want %v", tt.pkgPath, got, tt.want)
			}
		})
	}
}
