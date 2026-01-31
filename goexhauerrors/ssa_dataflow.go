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
	pass                *analysis.Pass
	ssaResult           *buildssa.SSA
	localErrs           *localErrors
	localFacts          map[*types.Func]*FunctionErrorsFact
	localParamFlowFacts map[*types.Func]*ParameterFlowFact
	interfaceImpls      *interfaceImplementations
	ssaFuncMap          map[*types.Func]*ssa.Function
}

// buildSSAFuncMap builds a lookup map from types.Func to ssa.Function for O(1) lookups.
func buildSSAFuncMap(ssaResult *buildssa.SSA) map[*types.Func]*ssa.Function {
	ssaFuncMap := make(map[*types.Func]*ssa.Function, len(ssaResult.SrcFuncs))
	for _, ssaFn := range ssaResult.SrcFuncs {
		if obj := ssaFn.Object(); obj != nil {
			if fn, ok := obj.(*types.Func); ok {
				ssaFuncMap[fn] = ssaFn
			}
		}
	}
	return ssaFuncMap
}

// newSSAAnalyzer creates a new SSA analyzer.
func newSSAAnalyzer(pass *analysis.Pass, localErrs *localErrors, localFacts map[*types.Func]*FunctionErrorsFact, localParamFlowFacts map[*types.Func]*ParameterFlowFact, interfaceImpls *interfaceImplementations, ssaFuncMap map[*types.Func]*ssa.Function) *ssaAnalyzer {
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	return &ssaAnalyzer{
		pass:                pass,
		ssaResult:           ssaResult,
		localErrs:           localErrs,
		localFacts:          localFacts,
		localParamFlowFacts: localParamFlowFacts,
		interfaceImpls:      interfaceImpls,
		ssaFuncMap:          ssaFuncMap,
	}
}

// findSSAFunction finds the SSA function corresponding to a types.Func.
func (a *ssaAnalyzer) findSSAFunction(fn *types.Func) *ssa.Function {
	return a.ssaFuncMap[fn]
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
		callErrs := a.getErrorsFromCall(v, visited, depth)
		errs = append(errs, callErrs...)

		// Also resolve errors via ParameterFlowFact
		paramFlowErrs := a.resolveParameterFlowErrors(v, visited, depth)
		errs = append(errs, paramFlowErrs...)
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

	// Filter out any external package types (including stdlib)
	return a.filterIgnoredPackages(errs)
}

// filterIgnoredPackages removes errors from ignored packages.
func (a *ssaAnalyzer) filterIgnoredPackages(errs []ErrorInfo) []ErrorInfo {
	var filtered []ErrorInfo
	for _, s := range errs {
		if !shouldIgnorePackage(s.PkgPath) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// getErrorsFromCall extracts error information from a function call.
// Only returns errors if the called function has FunctionErrorsFact.
func (a *ssaAnalyzer) getErrorsFromCall(call *ssa.Call, visited map[ssa.Value]bool, depth int) []ErrorInfo {
	// Check for interface method call (invoke mode)
	if call.Call.IsInvoke() {
		return a.getErrorsFromInvoke(call, visited, depth)
	}

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

	// Also resolve errors through ParameterFlowFact for static calls
	var flowFact *ParameterFlowFact
	if localPF, ok := a.localParamFlowFacts[typesFunc]; ok {
		flowFact = localPF
	}
	var importedPF ParameterFlowFact
	if a.pass.ImportObjectFact(typesFunc, &importedPF) {
		if flowFact == nil {
			flowFact = &importedPF
		}
	}
	if flowFact != nil {
		paramFlowErrs := a.resolveParameterFlowErrorsForStaticCall(call, flowFact, callee, visited, depth)
		errs = append(errs, paramFlowErrs...)
	}

	return errs
}

// getErrorsFromInvoke extracts error information from an interface method call.
// It collects errors from all known implementations of the interface method,
// and also resolves ParameterFlowFact to trace errors through parameters.
func (a *ssaAnalyzer) getErrorsFromInvoke(call *ssa.Call, visited map[ssa.Value]bool, depth int) []ErrorInfo {
	ifaceMethod := call.Call.Method
	if ifaceMethod == nil {
		return nil
	}

	var allErrors []ErrorInfo

	// Check InterfaceMethodFact for this method
	var ifaceFact InterfaceMethodFact
	hasIfaceFact := a.pass.ImportObjectFact(ifaceMethod, &ifaceFact)
	if hasIfaceFact {
		allErrors = append(allErrors, ifaceFact.Errors...)
	}

	// Check ParameterFlowFact on the interface method (exported as intersection of impls)
	var flowFact ParameterFlowFact
	if a.pass.ImportObjectFact(ifaceMethod, &flowFact) {
		paramFlowErrs := a.resolveParameterFlowErrorsForInvoke(call, &flowFact, visited, depth)
		allErrors = append(allErrors, paramFlowErrs...)
	}

	if hasIfaceFact {
		return a.deduplicateErrors(allErrors)
	}

	// Fallback: Get the interface type and scan implementations
	ifaceType := getInterfaceType(call.Call.Value.Type())
	if ifaceType == nil {
		return a.deduplicateErrors(allErrors)
	}

	// Find all implementations and collect their errors
	implementingTypes := a.interfaceImpls.getImplementingTypes(ifaceType)
	for _, concreteType := range implementingTypes {
		method := findMethodImplementation(concreteType, ifaceMethod)
		if method == nil {
			continue
		}

		// FunctionErrorsFact
		if localFact, ok := a.localFacts[method]; ok {
			allErrors = append(allErrors, localFact.Errors...)
		}
		var fnFact FunctionErrorsFact
		if a.pass.ImportObjectFact(method, &fnFact) {
			allErrors = append(allErrors, fnFact.Errors...)
		}
	}

	// If no ParameterFlowFact was found on the interface method, compute intersection from impls
	if !a.pass.ImportObjectFact(ifaceMethod, &flowFact) && len(implementingTypes) > 0 {
		var implFlowFacts []*ParameterFlowFact
		for _, concreteType := range implementingTypes {
			method := findMethodImplementation(concreteType, ifaceMethod)
			if method == nil {
				continue
			}
			var pf *ParameterFlowFact
			if localPF, ok := a.localParamFlowFacts[method]; ok {
				pf = localPF
			}
			var importedPF ParameterFlowFact
			if a.pass.ImportObjectFact(method, &importedPF) {
				if pf == nil {
					pf = &importedPF
				} else {
					pf.Merge(&importedPF)
				}
			}
			implFlowFacts = append(implFlowFacts, pf)
		}
		intersected := intersectParameterFlowFacts(implFlowFacts)
		if intersected != nil {
			paramFlowErrs := a.resolveParameterFlowErrorsForInvoke(call, intersected, visited, depth)
			allErrors = append(allErrors, paramFlowErrs...)
		}
	}

	return a.deduplicateErrors(allErrors)
}

// resolveParameterFlowErrorsForInvoke resolves concrete errors passed as arguments
// to an interface method call (invoke mode) based on ParameterFlowFact.
// In invoke mode, call.Call.Args contains only method arguments (no receiver).
func (a *ssaAnalyzer) resolveParameterFlowErrorsForInvoke(call *ssa.Call, flowFact *ParameterFlowFact, visited map[ssa.Value]bool, depth int) []ErrorInfo {
	var errs []ErrorInfo
	args := call.Call.Args

	for _, flow := range flowFact.Flows {
		argIdx := flow.ParamIndex
		if argIdx >= len(args) {
			continue
		}

		argErrs := a.traceValueToErrors(args[argIdx], visited, depth+1)
		for i := range argErrs {
			if flow.Wrapped {
				argErrs[i].Wrapped = true
			}
		}
		errs = append(errs, argErrs...)
	}

	return errs
}

// resolveParameterFlowErrorsForStaticCall resolves concrete errors passed as arguments
// to a static function/method call based on ParameterFlowFact.
// For method calls, args[0] is the receiver, so ParamIndex must be offset by 1.
func (a *ssaAnalyzer) resolveParameterFlowErrorsForStaticCall(call *ssa.Call, flowFact *ParameterFlowFact, callee *ssa.Function, visited map[ssa.Value]bool, depth int) []ErrorInfo {
	var errs []ErrorInfo
	args := call.Call.Args

	// For methods, SSA args include the receiver at index 0
	receiverOffset := 0
	sig := callee.Signature
	if sig.Recv() != nil {
		receiverOffset = 1
	}

	for _, flow := range flowFact.Flows {
		argIdx := flow.ParamIndex + receiverOffset
		if argIdx >= len(args) {
			continue
		}

		argErrs := a.traceValueToErrors(args[argIdx], visited, depth+1)
		for i := range argErrs {
			if flow.Wrapped {
				argErrs[i].Wrapped = true
			}
		}
		errs = append(errs, argErrs...)
	}

	return errs
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

	// Skip ignored packages
	if shouldIgnorePackage(pkg.Path()) {
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

	// Skip ignored packages
	if shouldIgnorePackage(pkg.Path()) {
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

	// Skip ignored packages
	if shouldIgnorePackage(pkg.Path()) {
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

// resolveParameterFlowErrors resolves concrete errors passed to functions with ParameterFlowFact.
func (a *ssaAnalyzer) resolveParameterFlowErrors(call *ssa.Call, visited map[ssa.Value]bool, depth int) []ErrorInfo {
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

	// Check for ParameterFlowFact
	var flowFact *ParameterFlowFact
	hasFlowFact := false

	// Check local facts first
	if localFlowFact, ok := a.localParamFlowFacts[typesFunc]; ok {
		flowFact = localFlowFact
		hasFlowFact = true
	}

	// Also check imported facts
	var importedFlowFact ParameterFlowFact
	if a.pass.ImportObjectFact(typesFunc, &importedFlowFact) {
		if flowFact == nil {
			flowFact = &importedFlowFact
		} else {
			flowFact.Merge(&importedFlowFact)
		}
		hasFlowFact = true
	}

	if !hasFlowFact || flowFact == nil || len(flowFact.Flows) == 0 {
		return nil
	}

	var errs []ErrorInfo
	args := call.Call.Args

	for _, flow := range flowFact.Flows {
		argIdx := flow.ParamIndex
		if argIdx >= len(args) {
			continue
		}

		// Recursively trace the argument to find concrete errors
		argErrs := a.traceValueToErrors(args[argIdx], visited, depth+1)

		// Apply wrapped flag if the parameter flow is wrapped
		for i := range argErrs {
			if flow.Wrapped {
				argErrs[i].Wrapped = true
			}
		}

		errs = append(errs, argErrs...)
	}

	return errs
}

// detectParameterFlow analyzes a function to determine which error parameters
// flow to return values.
func (a *ssaAnalyzer) detectParameterFlow(fn *types.Func, errorPositions []int) *ParameterFlowFact {
	ssaFn := a.findSSAFunction(fn)
	if ssaFn == nil {
		return nil
	}

	// Find error parameters
	sig := fn.Type().(*types.Signature)
	errorParamIndices := findErrorParamIndices(sig)
	if len(errorParamIndices) == 0 {
		return nil
	}

	// For methods, SSA includes receiver at index 0, but call.Args doesn't
	// We need to adjust the parameter index
	receiverOffset := 0
	if sig.Recv() != nil {
		receiverOffset = 1
	}

	fact := &ParameterFlowFact{}

	// For each return statement, trace which parameters reach it
	for _, block := range ssaFn.Blocks {
		for _, instr := range block.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}

			for _, pos := range errorPositions {
				if pos < len(ret.Results) {
					flows := a.traceValueToParameters(ret.Results[pos], ssaFn.Params, make(map[ssa.Value]bool), 0)
					for _, flow := range flows {
						// Adjust parameter index for methods (receiver is not in call.Args)
						adjustedIndex := flow.ParamIndex - receiverOffset
						if adjustedIndex < 0 {
							continue
						}
						adjustedFlow := ParameterFlowInfo{
							ParamIndex: adjustedIndex,
							Wrapped:    flow.Wrapped,
						}
						fact.AddFlow(adjustedFlow)
					}
				}
			}
		}
	}

	if len(fact.Flows) == 0 {
		return nil
	}
	return fact
}

// detectFunctionParamCallFlow analyzes a function to determine which function-typed
// parameters are called and their results flow to the error return.
// Example: func RunInTx(fn func() error) error { return fn() }
func (a *ssaAnalyzer) detectFunctionParamCallFlow(fn *types.Func, errorPositions []int) *FunctionParamCallFlowFact {
	ssaFn := a.findSSAFunction(fn)
	if ssaFn == nil {
		return nil
	}

	// For methods, SSA includes receiver at index 0, but call.Args doesn't
	// We need to adjust the parameter index
	sig := fn.Type().(*types.Signature)
	receiverOffset := 0
	if sig.Recv() != nil {
		receiverOffset = 1
	}

	fact := &FunctionParamCallFlowFact{}

	// For each return statement, trace if the return value comes from calling a function parameter
	for _, block := range ssaFn.Blocks {
		for _, instr := range block.Instrs {
			ret, ok := instr.(*ssa.Return)
			if !ok {
				continue
			}

			for _, pos := range errorPositions {
				if pos < len(ret.Results) {
					flows := a.traceValueToFunctionParamCalls(ret.Results[pos], ssaFn.Params, make(map[ssa.Value]bool), 0)
					for _, flow := range flows {
						// Adjust parameter index for methods (receiver is not in call.Args)
						adjustedIndex := flow.ParamIndex - receiverOffset
						if adjustedIndex < 0 {
							continue
						}
						adjustedFlow := FunctionParamCallFlowInfo{
							ParamIndex: adjustedIndex,
							Wrapped:    flow.Wrapped,
						}
						fact.AddCallFlow(adjustedFlow)
					}
				}
			}
		}
	}

	if len(fact.CallFlows) == 0 {
		return nil
	}
	return fact
}

// traceValueToFunctionParamCalls traces an SSA value to find if it comes from
// calling a function-typed parameter.
func (a *ssaAnalyzer) traceValueToFunctionParamCalls(val ssa.Value, params []*ssa.Parameter, visited map[ssa.Value]bool, depth int) []FunctionParamCallFlowInfo {
	if val == nil || visited[val] || depth > maxTraceDepth {
		return nil
	}
	visited[val] = true

	var flows []FunctionParamCallFlowInfo

	switch v := val.(type) {
	case *ssa.Call:
		// Check if the callee is a parameter (dynamic call to function parameter)
		if param, ok := v.Call.Value.(*ssa.Parameter); ok {
			// Find the parameter index
			for i, p := range params {
				if p == param {
					flows = append(flows, FunctionParamCallFlowInfo{
						ParamIndex: i,
						Wrapped:    false,
					})
				}
			}
		}

		// Also check for fmt.Errorf wrapping the result of a function parameter call
		callee := v.Call.StaticCallee()
		if callee != nil && isFmtErrorfSSA(callee) {
			wrappedFlows := a.analyzeErrorfWrappingForFunctionParamCalls(v, params, visited, depth)
			flows = append(flows, wrappedFlows...)
		}

	case *ssa.Phi:
		// Merge of values from different branches
		for _, edge := range v.Edges {
			flows = append(flows, a.traceValueToFunctionParamCalls(edge, params, visited, depth+1)...)
		}

	case *ssa.ChangeInterface:
		flows = append(flows, a.traceValueToFunctionParamCalls(v.X, params, visited, depth+1)...)

	case *ssa.MakeInterface:
		flows = append(flows, a.traceValueToFunctionParamCalls(v.X, params, visited, depth+1)...)

	case *ssa.Extract:
		flows = append(flows, a.traceValueToFunctionParamCalls(v.Tuple, params, visited, depth+1)...)
	}

	return deduplicateFunctionParamCallFlows(flows)
}

// analyzeErrorfWrappingForFunctionParamCalls checks if fmt.Errorf wraps the result of a function parameter call
func (a *ssaAnalyzer) analyzeErrorfWrappingForFunctionParamCalls(call *ssa.Call, params []*ssa.Parameter, visited map[ssa.Value]bool, depth int) []FunctionParamCallFlowInfo {
	variadicArgs, wrapIndices := getWrappedArgIndices(call)
	if len(wrapIndices) == 0 {
		return nil
	}

	var flows []FunctionParamCallFlowInfo
	for _, wrapIdx := range wrapIndices {
		if wrapIdx >= len(variadicArgs) {
			continue
		}

		paramCallFlows := a.traceValueToFunctionParamCalls(variadicArgs[wrapIdx], params, visited, depth+1)
		for i := range paramCallFlows {
			paramCallFlows[i].Wrapped = true
		}
		flows = append(flows, paramCallFlows...)
	}

	return flows
}

// deduplicateFunctionParamCallFlows removes duplicate function parameter call flows.
func deduplicateFunctionParamCallFlows(flows []FunctionParamCallFlowInfo) []FunctionParamCallFlowInfo {
	seen := make(map[int]bool)
	var result []FunctionParamCallFlowInfo
	for _, flow := range flows {
		if !seen[flow.ParamIndex] {
			seen[flow.ParamIndex] = true
			result = append(result, flow)
		}
	}
	return result
}

// traceValueToParameters traces an SSA value back to see if it originates from parameters.
// Returns a list of ParameterFlowInfo for any parameters that flow to this value.
func (a *ssaAnalyzer) traceValueToParameters(val ssa.Value, params []*ssa.Parameter, visited map[ssa.Value]bool, depth int) []ParameterFlowInfo {
	if val == nil || visited[val] || depth > maxTraceDepth {
		return nil
	}
	visited[val] = true

	var flows []ParameterFlowInfo

	switch v := val.(type) {
	case *ssa.Parameter:
		// Found a parameter - check if it's an error type
		for i, param := range params {
			if param == v && isErrorType(param.Type()) {
				flows = append(flows, ParameterFlowInfo{
					ParamIndex: i,
					Wrapped:    false,
				})
			}
		}

	case *ssa.Phi:
		// Merge of values from different branches
		for _, edge := range v.Edges {
			flows = append(flows, a.traceValueToParameters(edge, params, visited, depth+1)...)
		}

	case *ssa.Call:
		// Check if this is fmt.Errorf with %w wrapping a parameter
		callee := v.Call.StaticCallee()
		if callee != nil {
			if isFmtErrorfSSA(callee) {
				wrappedFlows := a.analyzeErrorfWrapping(v, params, visited, depth)
				flows = append(flows, wrappedFlows...)
			} else {
				// Check if the called function has ParameterFlowFact
				// and resolve transitively (wrapper calling another wrapper)
				transitiveFlows := a.traceTransitiveParameterFlow(v, params, visited, depth)
				flows = append(flows, transitiveFlows...)
			}
		}

	case *ssa.ChangeInterface:
		flows = append(flows, a.traceValueToParameters(v.X, params, visited, depth+1)...)

	case *ssa.MakeInterface:
		flows = append(flows, a.traceValueToParameters(v.X, params, visited, depth+1)...)

	case *ssa.Extract:
		flows = append(flows, a.traceValueToParameters(v.Tuple, params, visited, depth+1)...)
	}

	return deduplicateFlows(flows)
}

// traceTransitiveParameterFlow handles wrappers that call other wrappers.
func (a *ssaAnalyzer) traceTransitiveParameterFlow(call *ssa.Call, params []*ssa.Parameter, visited map[ssa.Value]bool, depth int) []ParameterFlowInfo {
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

	// Get ParameterFlowFact for the called function
	var flowFact *ParameterFlowFact
	if localFlowFact, ok := a.localParamFlowFacts[typesFunc]; ok {
		flowFact = localFlowFact
	}
	var importedFlowFact ParameterFlowFact
	if a.pass.ImportObjectFact(typesFunc, &importedFlowFact) {
		if flowFact == nil {
			flowFact = &importedFlowFact
		}
	}

	if flowFact == nil || len(flowFact.Flows) == 0 {
		return nil
	}

	var flows []ParameterFlowInfo
	args := call.Call.Args

	for _, flow := range flowFact.Flows {
		argIdx := flow.ParamIndex
		if argIdx >= len(args) {
			continue
		}

		// Trace the argument to see if it's a parameter
		argFlows := a.traceValueToParameters(args[argIdx], params, visited, depth+1)
		for i := range argFlows {
			if flow.Wrapped {
				argFlows[i].Wrapped = true
			}
		}
		flows = append(flows, argFlows...)
	}

	return flows
}

// isFmtErrorfSSA checks if the callee is fmt.Errorf
func isFmtErrorfSSA(callee *ssa.Function) bool {
	if callee.Pkg == nil {
		return false
	}
	return callee.Pkg.Pkg.Path() == "fmt" && callee.Name() == "Errorf"
}

// getWrappedArgIndices extracts the wrapped argument indices from a fmt.Errorf call.
// This is the common logic for analyzing fmt.Errorf wrapping patterns.
func getWrappedArgIndices(call *ssa.Call) (variadicArgs []ssa.Value, wrapIndices []int) {
	args := call.Call.Args
	if len(args) < 1 {
		return nil, nil
	}

	formatStr := extractConstantString(args[0])
	if formatStr == "" {
		return nil, nil
	}

	wrapIndices = findWrapVerbIndices(formatStr)
	if len(wrapIndices) == 0 {
		return nil, nil
	}

	// Handle variadic arguments - they may be passed as a slice
	variadicArgs = args[1:]
	if len(args) == 2 {
		if slice, ok := args[1].(*ssa.Slice); ok {
			variadicArgs = extractSliceElements(slice)
		}
	}

	return variadicArgs, wrapIndices
}

// analyzeErrorfWrapping analyzes fmt.Errorf calls for %w verbs that wrap parameters
func (a *ssaAnalyzer) analyzeErrorfWrapping(call *ssa.Call, params []*ssa.Parameter, visited map[ssa.Value]bool, depth int) []ParameterFlowInfo {
	variadicArgs, wrapIndices := getWrappedArgIndices(call)
	if len(wrapIndices) == 0 {
		return nil
	}

	var flows []ParameterFlowInfo
	for _, wrapIdx := range wrapIndices {
		if wrapIdx >= len(variadicArgs) {
			continue
		}

		paramFlows := a.traceValueToParameters(variadicArgs[wrapIdx], params, visited, depth+1)
		for i := range paramFlows {
			paramFlows[i].Wrapped = true
		}
		flows = append(flows, paramFlows...)
	}

	return flows
}

// extractSliceElements extracts elements from a slice used for variadic arguments.
func extractSliceElements(slice *ssa.Slice) []ssa.Value {
	var elements []ssa.Value

	// The slice is typically created from an array: slice t0[:]
	// where t0 is an Alloc for the array
	alloc, ok := slice.X.(*ssa.Alloc)
	if !ok {
		return nil
	}

	// Collect elements stored in the array by finding IndexAddr + Store patterns
	type indexedValue struct {
		index int64
		value ssa.Value
	}
	var indexed []indexedValue

	for _, ref := range *alloc.Referrers() {
		if idxAddr, ok := ref.(*ssa.IndexAddr); ok {
			// Get the constant index
			if constIdx, ok := idxAddr.Index.(*ssa.Const); ok {
				idx := constIdx.Int64()
				// Find the Store instruction that writes to this index
				for _, storeRef := range *idxAddr.Referrers() {
					if store, ok := storeRef.(*ssa.Store); ok {
						indexed = append(indexed, indexedValue{index: idx, value: store.Val})
					}
				}
			}
		}
	}

	// Sort by index and extract values
	// For simplicity, just add them in order found (usually correct for small arrays)
	for _, iv := range indexed {
		// Expand to the correct position if needed
		for len(elements) <= int(iv.index) {
			elements = append(elements, nil)
		}
		elements[iv.index] = iv.value
	}

	return elements
}

// extractConstantString extracts a constant string value from an SSA value.
func extractConstantString(val ssa.Value) string {
	if c, ok := val.(*ssa.Const); ok {
		if c.Value != nil {
			// Kind is "String" (capital S)
			if c.Value.Kind().String() == "String" {
				s := c.Value.ExactString()
				if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
					return s[1 : len(s)-1]
				}
			}
		}
	}
	return ""
}

// findErrorParamIndices finds which parameter indices are error types
func findErrorParamIndices(sig *types.Signature) []int {
	var indices []int
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		if isErrorType(params.At(i).Type()) {
			indices = append(indices, i)
		}
	}
	return indices
}

// deduplicateFlows removes duplicate parameter flows.
func deduplicateFlows(flows []ParameterFlowInfo) []ParameterFlowInfo {
	seen := make(map[int]bool)
	var result []ParameterFlowInfo
	for _, flow := range flows {
		if !seen[flow.ParamIndex] {
			seen[flow.ParamIndex] = true
			result = append(result, flow)
		}
	}
	return result
}
