package goexhauerrors

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// ssaAnalyzer provides SSA-based dataflow analysis for tracking error values.
type ssaAnalyzer struct {
	pass       *analysis.Pass
	ssaResult  *buildssa.SSA
	sentinels  *localSentinels
	localFacts map[*types.Func]*FunctionSentinelsFact
}

// newSSAAnalyzer creates a new SSA analyzer.
func newSSAAnalyzer(pass *analysis.Pass, sentinels *localSentinels, localFacts map[*types.Func]*FunctionSentinelsFact) *ssaAnalyzer {
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	return &ssaAnalyzer{
		pass:       pass,
		ssaResult:  ssaResult,
		sentinels:  sentinels,
		localFacts: localFacts,
	}
}

// findSSAFunction finds the SSA function corresponding to a types.Func.
func (a *ssaAnalyzer) findSSAFunction(fn *types.Func) *ssa.Function {
	for _, ssaFn := range a.ssaResult.SrcFuncs {
		if ssaFn.Object() == fn {
			return ssaFn
		}
	}
	return nil
}

// traceReturnStatements analyzes return statements in a function using SSA.
// It returns additional sentinels discovered through SSA analysis by tracking
// error values through local variables.
func (a *ssaAnalyzer) traceReturnStatements(fn *types.Func, errorPositions []int) []SentinelInfo {
	ssaFn := a.findSSAFunction(fn)
	if ssaFn == nil {
		return nil
	}

	var sentinels []SentinelInfo
	visited := make(map[ssa.Value]bool)

	// Iterate through all blocks and find Return instructions
	for _, block := range ssaFn.Blocks {
		for _, instr := range block.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}

			// Trace each error result
			for _, pos := range errorPositions {
				if pos < len(ret.Results) {
					tracedSentinels := a.traceValueToSentinels(ret.Results[pos], visited, 0)
					sentinels = append(sentinels, tracedSentinels...)
				}
			}
		}
	}

	return a.deduplicateSentinels(sentinels)
}

// maxTraceDepth limits recursion to prevent infinite loops and excessive tracing
const maxTraceDepth = 10

// traceValueToSentinels traces an SSA value back to its sentinel error sources.
// It only follows specific patterns that are known to propagate sentinel errors:
// - Function calls that have FunctionSentinelsFact
// - Phi nodes (conditional branches)
// - Extract (multi-return value)
// - Global variables that are sentinels
// - MakeInterface with known custom error types
func (a *ssaAnalyzer) traceValueToSentinels(val ssa.Value, visited map[ssa.Value]bool, depth int) []SentinelInfo {
	if val == nil || visited[val] || depth > maxTraceDepth {
		return nil
	}
	visited[val] = true

	var sentinels []SentinelInfo

	switch v := val.(type) {
	case *ssa.Call:
		// Function call result - get sentinels from the called function's facts only
		callSentinels := a.getSentinelsFromCall(v)
		sentinels = append(sentinels, callSentinels...)
		// Do NOT trace further into the call - this avoids picking up internal types

	case *ssa.Extract:
		// Extracting from a tuple (e.g., result of multi-return function)
		// The tuple comes from a Call, so trace it
		sentinels = append(sentinels, a.traceValueToSentinels(v.Tuple, visited, depth+1)...)

	case *ssa.Phi:
		// Phi node - merge of values from different branches
		for _, edge := range v.Edges {
			sentinels = append(sentinels, a.traceValueToSentinels(edge, visited, depth+1)...)
		}

	case *ssa.MakeInterface:
		// Converting concrete type to interface
		// Only add if it's a known custom error type (local or with fact)
		sentinels = append(sentinels, a.getSentinelsFromMakeInterface(v)...)
		// Do NOT trace v.X further to avoid discovering internal types

	case *ssa.UnOp:
		if v.Op == token.MUL { // Dereference (load from pointer)
			sentinels = append(sentinels, a.traceValueToSentinels(v.X, visited, depth+1)...)
		}

	case *ssa.Alloc:
		// Allocation - only add if it's a known custom error type
		sentinels = append(sentinels, a.getSentinelsFromAlloc(v)...)

	case *ssa.Global:
		// Global variable - check if it's a known sentinel
		sentinels = append(sentinels, a.getSentinelsFromGlobal(v)...)

	case *ssa.ChangeInterface:
		// Interface conversion - trace underlying value
		sentinels = append(sentinels, a.traceValueToSentinels(v.X, visited, depth+1)...)

	case *ssa.Parameter:
		// Function parameter - can't trace statically (known limitation)

	case *ssa.FieldAddr:
		// Field address - don't trace (known limitation)

	case *ssa.IndexAddr:
		// Index into slice/array - don't trace (known limitation)

	case *ssa.Lookup:
		// Map lookup - don't trace (known limitation)
	}

	// Filter out any standard library types that might have slipped through
	return filterStdlibSentinels(sentinels)
}

// filterStdlibSentinels removes sentinels from standard library packages.
func filterStdlibSentinels(sentinels []SentinelInfo) []SentinelInfo {
	var filtered []SentinelInfo
	for _, s := range sentinels {
		if !isStandardLibraryPackage(s.PkgPath) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// getSentinelsFromCall extracts sentinel information from a function call.
// Only returns sentinels if the called function has FunctionSentinelsFact.
func (a *ssaAnalyzer) getSentinelsFromCall(call *ssa.Call) []SentinelInfo {
	var sentinels []SentinelInfo

	callee := call.Call.StaticCallee()
	if callee == nil {
		return nil
	}

	fn := callee.Object()
	if fn == nil {
		return nil
	}

	typesFunc, ok := fn.(*types.Func)
	if !ok {
		return nil
	}

	// Check local facts first (for same-package functions)
	if localFact, ok := a.localFacts[typesFunc]; ok {
		sentinels = append(sentinels, localFact.Sentinels...)
	}

	// Also check imported facts (for cross-package or already exported)
	var fnFact FunctionSentinelsFact
	if a.pass.ImportObjectFact(typesFunc, &fnFact) {
		sentinels = append(sentinels, fnFact.Sentinels...)
	}

	return sentinels
}

// isStandardLibraryPackage checks if a package path is a standard library package
// that we should never consider for sentinel errors.
func isStandardLibraryPackage(pkgPath string) bool {
	// Standard library packages we should ignore
	stdlibPackages := map[string]bool{
		"errors": true,
		"fmt":    true,
		"io":     true,
		"os":     true,
		"net":    true,
		"time":   true,
	}
	return stdlibPackages[pkgPath]
}

// getSentinelsFromMakeInterface checks if a MakeInterface creates a known custom error type.
// Only returns sentinels if the type is explicitly registered as a sentinel type.
func (a *ssaAnalyzer) getSentinelsFromMakeInterface(v *ssa.MakeInterface) []SentinelInfo {
	var sentinels []SentinelInfo

	// Get the concrete type being converted to interface
	concreteType := v.X.Type()

	// Extract the named type (handling pointers)
	namedType := extractNamedType(concreteType)
	if namedType == nil {
		return nil
	}

	typeName := namedType.Obj()
	if typeName == nil {
		return nil
	}

	// Get the package
	pkg := typeName.Pkg()
	if pkg == nil {
		return nil
	}

	// Skip standard library packages entirely
	if isStandardLibraryPackage(pkg.Path()) {
		return nil
	}

	// Check if it's a local custom error type (same package)
	if pkg.Path() == a.pass.Pkg.Path() {
		if a.sentinels.types[typeName] {
			sentinels = append(sentinels, SentinelInfo{
				PkgPath: a.pass.Pkg.Path(),
				Name:    typeName.Name(),
				Wrapped: false,
			})
		}
		return sentinels
	}

	// For imported types, only use if they have an explicit fact
	var sentinelFact SentinelErrorFact
	if a.pass.ImportObjectFact(typeName, &sentinelFact) {
		sentinels = append(sentinels, SentinelInfo{
			PkgPath: sentinelFact.PkgPath,
			Name:    sentinelFact.Name,
			Wrapped: false,
		})
	}

	return sentinels
}

// getSentinelsFromAlloc checks if an Alloc creates a known custom error type.
func (a *ssaAnalyzer) getSentinelsFromAlloc(v *ssa.Alloc) []SentinelInfo {
	var sentinels []SentinelInfo

	allocType := v.Type()
	// Alloc returns a pointer
	ptrType, ok := allocType.(*types.Pointer)
	if !ok {
		return nil
	}

	namedType := extractNamedType(ptrType.Elem())
	if namedType == nil {
		return nil
	}

	typeName := namedType.Obj()
	if typeName == nil {
		return nil
	}

	// Get the package
	pkg := typeName.Pkg()
	if pkg == nil {
		return nil
	}

	// Skip standard library packages entirely
	if isStandardLibraryPackage(pkg.Path()) {
		return nil
	}

	// Check if it's a local custom error type (same package)
	if pkg.Path() == a.pass.Pkg.Path() {
		if a.sentinels.types[typeName] {
			sentinels = append(sentinels, SentinelInfo{
				PkgPath: a.pass.Pkg.Path(),
				Name:    typeName.Name(),
				Wrapped: false,
			})
		}
		return sentinels
	}

	// For imported types, only use if they have an explicit fact
	var sentinelFact SentinelErrorFact
	if a.pass.ImportObjectFact(typeName, &sentinelFact) {
		sentinels = append(sentinels, SentinelInfo{
			PkgPath: sentinelFact.PkgPath,
			Name:    sentinelFact.Name,
			Wrapped: false,
		})
	}

	return sentinels
}

// getSentinelsFromGlobal checks if a Global is a known sentinel error.
func (a *ssaAnalyzer) getSentinelsFromGlobal(v *ssa.Global) []SentinelInfo {
	var sentinels []SentinelInfo

	obj := v.Object()
	if obj == nil {
		return nil
	}

	varObj, ok := obj.(*types.Var)
	if !ok {
		return nil
	}

	// Get the package
	pkg := varObj.Pkg()
	if pkg == nil {
		return nil
	}

	// Skip standard library packages entirely
	if isStandardLibraryPackage(pkg.Path()) {
		return nil
	}

	// Check if it's a local sentinel (same package)
	if pkg.Path() == a.pass.Pkg.Path() {
		if a.sentinels.vars[varObj] {
			sentinels = append(sentinels, SentinelInfo{
				PkgPath: a.pass.Pkg.Path(),
				Name:    varObj.Name(),
				Wrapped: false,
			})
		}
		return sentinels
	}

	// For imported variables, only use if they have an explicit fact
	var sentinelFact SentinelErrorFact
	if a.pass.ImportObjectFact(varObj, &sentinelFact) {
		sentinels = append(sentinels, SentinelInfo{
			PkgPath: sentinelFact.PkgPath,
			Name:    sentinelFact.Name,
			Wrapped: false,
		})
	}

	return sentinels
}

// deduplicateSentinels removes duplicate sentinels from the list.
func (a *ssaAnalyzer) deduplicateSentinels(sentinels []SentinelInfo) []SentinelInfo {
	seen := make(map[string]bool)
	var result []SentinelInfo
	for _, s := range sentinels {
		key := s.Key()
		if !seen[key] {
			seen[key] = true
			result = append(result, s)
		}
	}
	return result
}
