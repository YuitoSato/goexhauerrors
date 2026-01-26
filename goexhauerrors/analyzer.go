package goexhauerrors

import (
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

var Analyzer = &analysis.Analyzer{
	Name: "exhaustiveerrors",
	Doc:  "checks that all error types returned by functions are exhaustively checked with errors.Is/As",
	Run:  run,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
		buildssa.Analyzer,
	},
	FactTypes: []analysis.Fact{
		(*ErrorFact)(nil),
		(*FunctionErrorsFact)(nil),
	},
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Phase 1: Detect local errors (sentinels and custom types) in this package and export facts
	localErrors := detectLocalErrors(pass)

	// Phase 2: Analyze function bodies for returns
	analyzeFunctionReturns(pass, localErrors)

	// Phase 2b: Analyze closures assigned to variables
	analyzeClosures(pass, localErrors)

	// Phase 3: Check call sites for exhaustive errors.Is checks
	checkCallSites(pass)

	return nil, nil
}

// localErrors holds local error information for the current package.
type localErrors struct {
	// vars maps *types.Var to true for error variables defined with errors.New() or fmt.Errorf()
	vars map[*types.Var]bool
	// types maps *types.TypeName to true for custom error types
	types map[*types.TypeName]bool
}

func newLocalErrors() *localErrors {
	return &localErrors{
		vars:  make(map[*types.Var]bool),
		types: make(map[*types.TypeName]bool),
	}
}
