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
		"crosspkg/errors",
		"crosspkg/middle",
		"crosspkg/caller",
	)
}
