package goexhauerrors_test

import (
	"testing"

	"github.com/YuitoSato/goexhauerrors/goexhauerrors"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, goexhauerrors.Analyzer,
		"basic",
		"customtype",
		"wrapped",
		"propagate",
		"method",
		"closure",
		"factory",
		"reassign",
		"conditional",
		"limitations",
		"nested",
		"unexported",
		"paramflow",
		"functiontype",
		"interfacecall",
		"higherorder",
		"crosspkg/errors",
		"crosspkg/middle",
		"crosspkg/caller",
		"crosspkgmethod/errors",
		"crosspkgmethod/usecase",
		"crosspkgmethod/presentation",
		"deferselect",
		"funclit",
		"ifaceparamflow",
		"ifacechecked",
		"crosspkgiface/iface",
		"crosspkgiface/impl",
		"crosspkgiface/caller",
		"crosspkgunexported/errors",
		"crosspkgunexported/caller",
	)
}

func TestAnalyzerWithIgnorePackages(t *testing.T) {
	testdata := analysistest.TestData()

	// Set the ignorePackages flag
	if err := goexhauerrors.Analyzer.Flags.Set("ignorePackages", "ignored"); err != nil {
		t.Fatalf("failed to set ignorePackages flag: %v", err)
	}

	// Reset flag after test
	defer func() {
		_ = goexhauerrors.Analyzer.Flags.Set("ignorePackages", "")
	}()

	// Run tests - errors from "ignored" package should be skipped in useignored
	analysistest.Run(t, testdata, goexhauerrors.Analyzer,
		"ignored",
		"useignored",
	)
}
