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
	localErrs  *localErrors
	localFacts map[*types.Func]*FunctionErrorsFact
}

// newSSAAnalyzer creates a new SSA analyzer.
func newSSAAnalyzer(pass *analysis.Pass, localErrs *localErrors, localFacts map[*types.Func]*FunctionErrorsFact) *ssaAnalyzer {
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	return &ssaAnalyzer{
		pass:       pass,
		ssaResult:  ssaResult,
		localErrs:  localErrs,
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
// It returns additional errors discovered through SSA analysis by tracking
// error values through local variables.
func (a *ssaAnalyzer) traceReturnStatements(fn *types.Func, errorPositions []int) []ErrorInfo {
	ssaFn := a.findSSAFunction(fn)
	if ssaFn == nil {
		return nil
	}

	var errs []ErrorInfo
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
					tracedErrs := a.traceValueToErrors(ret.Results[pos], visited, 0)
					errs = append(errs, tracedErrs...)
				}
			}
		}
	}

	return a.deduplicateErrors(errs)
}

// maxTraceDepth limits recursion to prevent infinite loops and excessive tracing
const maxTraceDepth = 10

// traceValueToErrors traces an SSA value back to its error sources.
// It only follows specific patterns that are known to propagate errors:
// - Function calls that have FunctionErrorsFact
// - Phi nodes (conditional branches)
// - Extract (multi-return value)
// - Global variables that are errors
// - MakeInterface with known custom error types
func (a *ssaAnalyzer) traceValueToErrors(val ssa.Value, visited map[ssa.Value]bool, depth int) []ErrorInfo {
	if val == nil || visited[val] || depth > maxTraceDepth {
		return nil
	}
	visited[val] = true

	var errs []ErrorInfo

	switch v := val.(type) {
	case *ssa.Call:
		// Function call result - get errors from the called function's facts only
		callErrs := a.getErrorsFromCall(v)
		errs = append(errs, callErrs...)
		// Do NOT trace further into the call - this avoids picking up internal types

	case *ssa.Extract:
		// Extracting from a tuple (e.g., result of multi-return function)
		// The tuple comes from a Call, so trace it
		errs = append(errs, a.traceValueToErrors(v.Tuple, visited, depth+1)...)

	case *ssa.Phi:
		// Phi node - merge of values from different branches
		for _, edge := range v.Edges {
			errs = append(errs, a.traceValueToErrors(edge, visited, depth+1)...)
		}

	case *ssa.MakeInterface:
		// Converting concrete type to interface
		// Only add if it's a known custom error type (local or with fact)
		errs = append(errs, a.getErrorsFromMakeInterface(v)...)
		// Do NOT trace v.X further to avoid discovering internal types

	case *ssa.UnOp:
		if v.Op == token.MUL { // Dereference (load from pointer)
			errs = append(errs, a.traceValueToErrors(v.X, visited, depth+1)...)
		}

	case *ssa.Alloc:
		// Allocation - only add if it's a known custom error type
		errs = append(errs, a.getErrorsFromAlloc(v)...)

	case *ssa.Global:
		// Global variable - check if it's a known error
		errs = append(errs, a.getErrorsFromGlobal(v)...)

	case *ssa.ChangeInterface:
		// Interface conversion - trace underlying value
		errs = append(errs, a.traceValueToErrors(v.X, visited, depth+1)...)

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
	return filterStdlibErrors(errs)
}

// filterStdlibErrors removes errors from standard library packages.
func filterStdlibErrors(errs []ErrorInfo) []ErrorInfo {
	var filtered []ErrorInfo
	for _, s := range errs {
		if !isStandardLibraryPackage(s.PkgPath) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// getErrorsFromCall extracts error information from a function call.
// Only returns errors if the called function has FunctionErrorsFact.
func (a *ssaAnalyzer) getErrorsFromCall(call *ssa.Call) []ErrorInfo {
	var errs []ErrorInfo

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
		errs = append(errs, localFact.Errors...)
	}

	// Also check imported facts (for cross-package or already exported)
	var fnFact FunctionErrorsFact
	if a.pass.ImportObjectFact(typesFunc, &fnFact) {
		errs = append(errs, fnFact.Errors...)
	}

	return errs
}

// isStandardLibraryPackage checks if a package path is a standard library package
// that we should never consider for errors.
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

// getErrorsFromMakeInterface checks if a MakeInterface creates a known custom error type.
// Only returns errors if the type is explicitly registered as a error type.
func (a *ssaAnalyzer) getErrorsFromMakeInterface(v *ssa.MakeInterface) []ErrorInfo {
	var errs []ErrorInfo

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
		if a.localErrs.types[typeName] {
			errs = append(errs, ErrorInfo{
				PkgPath: a.pass.Pkg.Path(),
				Name:    typeName.Name(),
				Wrapped: false,
			})
		}
		return errs
	}

	// For imported types, only use if they have an explicit fact
	var errorFact ErrorFact
	if a.pass.ImportObjectFact(typeName, &errorFact) {
		errs = append(errs, ErrorInfo{
			PkgPath: errorFact.PkgPath,
			Name:    errorFact.Name,
			Wrapped: false,
		})
	}

	return errs
}

// getErrorsFromAlloc checks if an Alloc creates a known custom error type.
func (a *ssaAnalyzer) getErrorsFromAlloc(v *ssa.Alloc) []ErrorInfo {
	var errs []ErrorInfo

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
		if a.localErrs.types[typeName] {
			errs = append(errs, ErrorInfo{
				PkgPath: a.pass.Pkg.Path(),
				Name:    typeName.Name(),
				Wrapped: false,
			})
		}
		return errs
	}

	// For imported types, only use if they have an explicit fact
	var errorFact ErrorFact
	if a.pass.ImportObjectFact(typeName, &errorFact) {
		errs = append(errs, ErrorInfo{
			PkgPath: errorFact.PkgPath,
			Name:    errorFact.Name,
			Wrapped: false,
		})
	}

	return errs
}

// getErrorsFromGlobal checks if a Global is a known error error.
func (a *ssaAnalyzer) getErrorsFromGlobal(v *ssa.Global) []ErrorInfo {
	var errs []ErrorInfo

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

	// Check if it's a local error (same package)
	if pkg.Path() == a.pass.Pkg.Path() {
		if a.localErrs.vars[varObj] {
			errs = append(errs, ErrorInfo{
				PkgPath: a.pass.Pkg.Path(),
				Name:    varObj.Name(),
				Wrapped: false,
			})
		}
		return errs
	}

	// For imported variables, only use if they have an explicit fact
	var errorFact ErrorFact
	if a.pass.ImportObjectFact(varObj, &errorFact) {
		errs = append(errs, ErrorInfo{
			PkgPath: errorFact.PkgPath,
			Name:    errorFact.Name,
			Wrapped: false,
		})
	}

	return errs
}

// deduplicateErrors removes duplicate errors from the list.
func (a *ssaAnalyzer) deduplicateErrors(errs []ErrorInfo) []ErrorInfo {
	seen := make(map[string]bool)
	var result []ErrorInfo
	for _, s := range errs {
		key := s.Key()
		if !seen[key] {
			seen[key] = true
			result = append(result, s)
		}
	}
	return result
}
