package checker_test

import (
	"testing"

	"github.com/YuitoSato/goexhauerrors/goexhauerrors/analyzer"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/checker"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/detector"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/facts"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/analysis/passes/inspect"
)

// testAnalyzer composes the full pipeline so that facts are properly exported
// before checker.CheckCallSites runs.
var testAnalyzer = &analysis.Analyzer{
	Name: "checkertest",
	Doc:  "test analyzer that runs the full pipeline for checker tests",
	Run:  runTestAnalyzer,
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

func runTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	// Phase 1: Detect local errors
	localErrors := detector.DetectLocalErrors(pass)

	// Phase 2: Analyze function bodies for returns
	localFacts, localParamFlowFacts, localCallFlowFacts, interfaceImpls := analyzer.AnalyzeFunctionReturns(pass, localErrors)

	// Phase 2b: Analyze closures
	analyzer.AnalyzeClosures(pass, localErrors)

	// Phase 2c: Analyze parameter error checks
	analyzer.AnalyzeParameterErrorChecks(pass)

	// Phase 2d: Compute interface method facts
	analyzer.ComputeInterfaceMethodFacts(pass, localFacts, localParamFlowFacts, localCallFlowFacts, interfaceImpls)

	// Phase 3: Check call sites (the phase under test)
	checker.CheckCallSites(pass, interfaceImpls)

	return nil, nil
}

func TestCheckerBasic(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, testAnalyzer, "basic")
}

func TestCheckerChecked(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, testAnalyzer, "checked")
}

func TestCheckerBranching(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, testAnalyzer, "branching")
}

func TestCheckerPropagation(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, testAnalyzer, "propagation")
}

func TestCheckerPartial(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, testAnalyzer, "partial")
}

func TestCheckerSwitchPropagation(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, testAnalyzer, "switch_propagation")
}
