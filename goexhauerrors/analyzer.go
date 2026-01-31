package goexhauerrors

import (
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/analyzer"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/checker"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/detector"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/facts"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/internal"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

// ignorePackages is a comma-separated list of package paths to ignore.
var ignorePackages string

func init() {
	Analyzer.Flags.StringVar(&ignorePackages, "ignorePackages", "",
		"comma-separated list of package paths to ignore (e.g., gorm.io/gorm,database/sql)")
}

var Analyzer = &analysis.Analyzer{
	Name: "exhaustiveerrors",
	Doc:  "checks that all error types returned by functions are exhaustively checked with errors.Is/As",
	Run:  run,
	Requires: []*analysis.Analyzer{
		inspect.Analyzer,
		buildssa.Analyzer,
	},
	FactTypes: []analysis.Fact{
		(*facts.ErrorFact)(nil),
		(*facts.FunctionErrorsFact)(nil),
		(*facts.ParameterFlowFact)(nil),
		(*facts.InterfaceMethodFact)(nil),
		(*facts.FunctionParamCallFlowFact)(nil),
		(*facts.ParameterCheckedErrorsFact)(nil),
	},
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Set ignore packages for the internal package
	internal.SetIgnorePackages(ignorePackages)

	// Phase 1: Detect local errors (sentinels and custom types) in this package and export facts
	localErrors := detector.DetectLocalErrors(pass)

	// Phase 2: Analyze function bodies for returns
	localFacts, localParamFlowFacts, interfaceImpls := analyzer.AnalyzeFunctionReturns(pass, localErrors)

	// Phase 2b: Analyze closures assigned to variables
	analyzer.AnalyzeClosures(pass, localErrors)

	// Phase 2c: Analyze errors.Is/As checks on function parameters
	analyzer.AnalyzeParameterErrorChecks(pass)

	// Phase 2d: Compute interface method facts (after ParameterCheckedErrorsFact is available)
	analyzer.ComputeInterfaceMethodFacts(pass, localFacts, localParamFlowFacts, interfaceImpls)

	// Phase 2e: Compute interface method facts for imported interfaces (DI pattern support)
	analyzer.ComputeImportedInterfaceMethodFacts(pass, localFacts, interfaceImpls)

	// Phase 3: Check call sites for exhaustive errors.Is checks
	checker.CheckCallSites(pass, interfaceImpls)

	// Phase 4: Process deferred function checks from earlier packages.
	// When a checker couldn't find interface method errors in the global store
	// (because the implementation package hadn't been analyzed yet), it registers
	// a deferred re-analysis callback. By now, this package's facts are in the
	// global store, so deferred functions may find what they need.
	facts.ProcessDeferredFunctionChecks()

	return nil, nil
}
