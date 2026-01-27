package goexhauerrors

import "testing"

func TestShouldIgnorePackage(t *testing.T) {
	tests := []struct {
		name              string
		ignorePackagesVal string
		pkgPath           string
		want              bool
	}{
		{"empty ignore list", "", "github.com/user/repo", false},
		{"single match", "gorm.io/gorm", "gorm.io/gorm", true},
		{"single no match", "gorm.io/gorm", "github.com/user/repo", false},
		{"multiple match first", "gorm.io/gorm,database/sql", "gorm.io/gorm", true},
		{"multiple match second", "gorm.io/gorm,database/sql", "database/sql", true},
		{"multiple no match", "gorm.io/gorm,database/sql", "net/http", false},
		{"with spaces", "gorm.io/gorm, database/sql", "database/sql", true},
		{"stdlib strconv", "strconv", "strconv", true},
		{"partial match should not match", "gorm.io/gorm", "gorm.io/gormfoo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set global ignorePackages for this test
			oldVal := ignorePackages
			ignorePackages = tt.ignorePackagesVal
			defer func() { ignorePackages = oldVal }()

			got := shouldIgnorePackage(tt.pkgPath)
			if got != tt.want {
				t.Errorf("shouldIgnorePackage(%q) with ignorePackages=%q = %v, want %v",
					tt.pkgPath, tt.ignorePackagesVal, got, tt.want)
			}
		})
	}
}
