package goexhauerrors

import (
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

var Analyzer = &analysis.Analyzer{
	Name: "exhaustiveerrors",
	Doc:  "checks that all error types (sentinel errors and custom error types) returned by functions are exhaustively checked with errors.Is/As",
	Run:  run,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
	},
	FactTypes: []analysis.Fact{
		(*SentinelErrorFact)(nil),
		(*FunctionSentinelsFact)(nil),
	},
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Phase 1: Detect sentinel errors in this package and export facts
	localSentinels := detectSentinelErrors(pass)

	// Phase 2: Analyze function bodies for sentinel returns
	analyzeFunctionReturns(pass, localSentinels)

	// Phase 2b: Analyze closures assigned to variables
	analyzeClosures(pass, localSentinels)

	// Phase 3: Check call sites for exhaustive errors.Is checks
	checkCallSites(pass)

	return nil, nil
}

// localSentinels holds sentinel error information for the current package.
type localSentinels struct {
	// vars maps *types.Var to true for sentinel error variables (var Err* = errors.New())
	vars map[*types.Var]bool
	// types maps *types.TypeName to true for custom error types
	types map[*types.TypeName]bool
}

func newLocalSentinels() *localSentinels {
	return &localSentinels{
		vars:  make(map[*types.Var]bool),
		types: make(map[*types.TypeName]bool),
	}
}
