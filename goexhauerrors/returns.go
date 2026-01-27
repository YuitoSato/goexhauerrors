package goexhauerrors

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// funcInfo holds information about a function for iterative analysis.
type funcInfo struct {
	fn             *types.Func
	body           *ast.BlockStmt
	errorPositions []int
}

// analyzeFunctionReturns analyzes all functions in the package and exports
// FunctionErrorsFact for each function that can return errors.
// It uses iterative analysis to handle factory functions that call other
// functions returning errors, combined with SSA-based analysis to track
// errors through variables.
func analyzeFunctionReturns(pass *analysis.Pass, localErrs *localErrors) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Discover interface implementations in this package
	interfaceImpls := findInterfaceImplementations(pass)

	// Collect all function declarations
	var funcs []funcInfo
	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		funcDecl := n.(*ast.FuncDecl)
		if funcDecl.Body == nil {
			return
		}

		funcObj := pass.TypesInfo.Defs[funcDecl.Name]
		if funcObj == nil {
			return
		}
		fn, ok := funcObj.(*types.Func)
		if !ok {
			return
		}

		sig := fn.Type().(*types.Signature)
		errorPositions := findErrorReturnPositions(sig)
		if len(errorPositions) == 0 {
			return
		}

		funcs = append(funcs, funcInfo{
			fn:             fn,
			body:           funcDecl.Body,
			errorPositions: errorPositions,
		})
	})

	// Track facts for local functions during iteration
	localFacts := make(map[*types.Func]*FunctionErrorsFact)
	localParamFlowFacts := make(map[*types.Func]*ParameterFlowFact)
	localCallFlowFacts := make(map[*types.Func]*FunctionParamCallFlowFact)

	// Iterate until no new facts are discovered (AST-based + SSA-based analysis)
	for {
		changed := false

		// Create SSA analyzer with current local facts
		ssaAnalyzer := newSSAAnalyzer(pass, localErrs, localFacts, localParamFlowFacts, interfaceImpls)

		for _, fi := range funcs {
			// Phase A: Detect parameter flow (for error-typed parameters)
			paramFlowFact := ssaAnalyzer.detectParameterFlow(fi.fn, fi.errorPositions)
			if paramFlowFact != nil {
				existingParamFlow := localParamFlowFacts[fi.fn]
				if existingParamFlow == nil {
					localParamFlowFacts[fi.fn] = paramFlowFact
					changed = true
				} else {
					oldLen := len(existingParamFlow.Flows)
					existingParamFlow.Merge(paramFlowFact)
					if len(existingParamFlow.Flows) > oldLen {
						changed = true
					}
				}
			}

			// Phase A2: Detect function parameter call flow (for function-typed parameters)
			callFlowFact := ssaAnalyzer.detectFunctionParamCallFlow(fi.fn, fi.errorPositions)
			if callFlowFact != nil {
				existingCallFlow := localCallFlowFacts[fi.fn]
				if existingCallFlow == nil {
					localCallFlowFacts[fi.fn] = callFlowFact
					changed = true
				} else {
					oldLen := len(existingCallFlow.CallFlows)
					existingCallFlow.Merge(callFlowFact)
					if len(existingCallFlow.CallFlows) > oldLen {
						changed = true
					}
				}
			}

			// Phase B: Analyze errors (AST-based + SSA-based)
			fact := &FunctionErrorsFact{}

			// AST-based analysis
			analyzeReturnsWithLocalFacts(pass, fi.body, fi.errorPositions, localErrs, fact, localFacts)

			// SSA-based analysis to trace errors through variables
			ssaErrors := ssaAnalyzer.traceReturnStatements(fi.fn, fi.errorPositions)
			for _, s := range ssaErrors {
				fact.AddError(s)
			}

			existing := localFacts[fi.fn]
			if existing == nil {
				if len(fact.Errors) > 0 {
					localFacts[fi.fn] = fact
					changed = true
				}
			} else {
				// Check if we found new errors
				oldLen := len(existing.Errors)
				existing.Merge(fact)
				if len(existing.Errors) > oldLen {
					changed = true
				}
			}
		}

		if !changed {
			break
		}
	}

	// Build set of valid errors (local + imported with facts)
	validErrors := buildValidErrors(pass, localErrs)

	// Export all discovered facts, filtering out invalid errors
	for fn, fact := range localFacts {
		fact.FilterByValidErrors(validErrors)
		if len(fact.Errors) > 0 {
			pass.ExportObjectFact(fn, fact)
		}
	}

	// Export ParameterFlowFact for cross-package usage
	for fn, fact := range localParamFlowFacts {
		if len(fact.Flows) > 0 {
			pass.ExportObjectFact(fn, fact)
		}
	}

	// Export FunctionParamCallFlowFact for cross-package usage
	for fn, fact := range localCallFlowFacts {
		if len(fact.CallFlows) > 0 {
			pass.ExportObjectFact(fn, fact)
		}
	}

	// Compute and export InterfaceMethodFact for interface methods
	computeInterfaceMethodFacts(pass, localFacts, interfaceImpls)
}

// buildValidErrors creates a set of valid error keys that can be used in FunctionErrorsFact.
// A error is valid if:
// 1. It's a local error (var or type) in the current package
// 2. It has an imported ErrorFact from the same module (not external packages)
func buildValidErrors(pass *analysis.Pass, localErrs *localErrors) map[string]bool {
	valid := make(map[string]bool)
	modulePath := getModulePath(pass)

	// Add local error variables
	for varObj := range localErrs.vars {
		key := pass.Pkg.Path() + "." + varObj.Name()
		valid[key] = true
	}

	// Add local custom error types
	for typeName := range localErrs.types {
		key := pass.Pkg.Path() + "." + typeName.Name()
		valid[key] = true
	}

	// Note: Imported errors are validated by checking ImportObjectFact during analysis
	// We need to also allow imported errors that have facts
	// This is done by scanning all referenced packages for exported facts
	// Only include errors from the same module (not external packages)
	for _, imp := range pass.Pkg.Imports() {
		// Skip external packages (not in the same module)
		if !isModulePackage(modulePath, imp.Path()) {
			continue
		}

		scope := imp.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			// Check for error variable facts
			if varObj, ok := obj.(*types.Var); ok {
				var errorFact ErrorFact
				if pass.ImportObjectFact(varObj, &errorFact) {
					valid[errorFact.Key()] = true
				}
			}
			// Check for error type facts
			if typeName, ok := obj.(*types.TypeName); ok {
				var errorFact ErrorFact
				if pass.ImportObjectFact(typeName, &errorFact) {
					valid[errorFact.Key()] = true
				}
			}
		}
	}

	return valid
}

// analyzeReturnsWithLocalFacts is like analyzeReturns but also checks local function facts.
func analyzeReturnsWithLocalFacts(pass *analysis.Pass, body *ast.BlockStmt, errorPositions []int, localErrs *localErrors, fact *FunctionErrorsFact, localFacts map[*types.Func]*FunctionErrorsFact) {
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		for _, pos := range errorPositions {
			if pos < len(ret.Results) {
				analyzeErrorExprWithLocalFacts(pass, ret.Results[pos], localErrs, fact, false, localFacts)
			}
		}

		return true
	})
}

// analyzeErrorExprWithLocalFacts is like analyzeErrorExpr but also checks local function facts.
func analyzeErrorExprWithLocalFacts(pass *analysis.Pass, expr ast.Expr, localErrs *localErrors, fact *FunctionErrorsFact, wrapped bool, localFacts map[*types.Func]*FunctionErrorsFact) {
	switch e := expr.(type) {
	case *ast.Ident:
		obj := pass.TypesInfo.Uses[e]
		if obj == nil {
			return
		}

		if varObj, ok := obj.(*types.Var); ok {
			if localErrs.vars[varObj] {
				fact.AddError(ErrorInfo{
					PkgPath: pass.Pkg.Path(),
					Name:    varObj.Name(),
					Wrapped: wrapped,
				})
				return
			}
			var errorFact ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				fact.AddError(ErrorInfo{
					PkgPath: errorFact.PkgPath,
					Name:    errorFact.Name,
					Wrapped: wrapped,
				})
			}
		}

	case *ast.SelectorExpr:
		obj := pass.TypesInfo.Uses[e.Sel]
		if obj == nil {
			return
		}

		if varObj, ok := obj.(*types.Var); ok {
			var errorFact ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				fact.AddError(ErrorInfo{
					PkgPath: errorFact.PkgPath,
					Name:    errorFact.Name,
					Wrapped: wrapped,
				})
			}
		}

	case *ast.CallExpr:
		if isFmtErrorfCall(pass, e) {
			analyzeFmtErrorfCallWithLocalFacts(pass, e, localErrs, fact, localFacts)
			return
		}

		if compLit := extractCompositeLit(e); compLit != nil {
			analyzeCompositeLit(pass, compLit, localErrs, fact, wrapped)
			return
		}

		// Check for function calls - first check local facts, then imported facts
		calledFn := getCalledFunction(pass, e)
		if calledFn != nil {
			// Check local facts first (for same-package functions)
			if localFact, ok := localFacts[calledFn]; ok {
				fact.Merge(localFact)
			}
			// Also check imported facts (for cross-package or already exported)
			var fnFact FunctionErrorsFact
			if pass.ImportObjectFact(calledFn, &fnFact) {
				fact.Merge(&fnFact)
			}
		}

	case *ast.UnaryExpr:
		if e.Op.String() == "&" {
			if compLit, ok := e.X.(*ast.CompositeLit); ok {
				analyzeCompositeLit(pass, compLit, localErrs, fact, wrapped)
			}
		}

	case *ast.CompositeLit:
		analyzeCompositeLit(pass, e, localErrs, fact, wrapped)
	}
}

// analyzeFmtErrorfCallWithLocalFacts is like analyzeFmtErrorfCall but uses local facts.
func analyzeFmtErrorfCallWithLocalFacts(pass *analysis.Pass, call *ast.CallExpr, localErrs *localErrors, fact *FunctionErrorsFact, localFacts map[*types.Func]*FunctionErrorsFact) {
	if len(call.Args) < 1 {
		return
	}

	formatStr := extractStringLiteral(call.Args[0])
	if formatStr == "" {
		return
	}

	wrapIndices := findWrapVerbIndices(formatStr)
	if len(wrapIndices) == 0 {
		return
	}

	for _, wrapIdx := range wrapIndices {
		argIdx := 1 + wrapIdx
		if argIdx >= len(call.Args) {
			continue
		}
		analyzeErrorExprWithLocalFacts(pass, call.Args[argIdx], localErrs, fact, true, localFacts)
	}
}

// findErrorReturnPositions finds which return value positions are error type.
func findErrorReturnPositions(sig *types.Signature) []int {
	var positions []int
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		if isErrorType(results.At(i).Type()) {
			positions = append(positions, i)
		}
	}
	return positions
}

// analyzeReturns walks through the function body and analyzes return statements.
func analyzeReturns(pass *analysis.Pass, body *ast.BlockStmt, errorPositions []int, localErrs *localErrors, fact *FunctionErrorsFact) {
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		// Analyze each error return position
		for _, pos := range errorPositions {
			if pos < len(ret.Results) {
				analyzeErrorExpr(pass, ret.Results[pos], localErrs, fact, false)
			}
		}

		return true
	})
}

// analyzeErrorExpr analyzes an expression to find errors.
func analyzeErrorExpr(pass *analysis.Pass, expr ast.Expr, localErrs *localErrors, fact *FunctionErrorsFact, wrapped bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		// Direct error return: return ErrNotFound
		obj := pass.TypesInfo.Uses[e]
		if obj == nil {
			return
		}

		if varObj, ok := obj.(*types.Var); ok {
			// Check local errors
			if localErrs.vars[varObj] {
				fact.AddError(ErrorInfo{
					PkgPath: pass.Pkg.Path(),
					Name:    varObj.Name(),
					Wrapped: wrapped,
				})
				return
			}
			// Check imported error via fact
			var errorFact ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				fact.AddError(ErrorInfo{
					PkgPath: errorFact.PkgPath,
					Name:    errorFact.Name,
					Wrapped: wrapped,
				})
			}
		}

	case *ast.SelectorExpr:
		// Qualified error: return pkg.ErrNotFound
		obj := pass.TypesInfo.Uses[e.Sel]
		if obj == nil {
			return
		}

		if varObj, ok := obj.(*types.Var); ok {
			var errorFact ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				fact.AddError(ErrorInfo{
					PkgPath: errorFact.PkgPath,
					Name:    errorFact.Name,
					Wrapped: wrapped,
				})
			}
		}

	case *ast.CallExpr:
		// Check for fmt.Errorf with %w wrapping a error
		if isFmtErrorfCall(pass, e) {
			analyzeFmtErrorfCall(pass, e, localErrs, fact)
			return
		}

		// Check for custom error type construction: &MyError{} or MyError{}
		if compLit := extractCompositeLit(e); compLit != nil {
			analyzeCompositeLit(pass, compLit, localErrs, fact, wrapped)
			return
		}

		// Check for function calls that might return errors
		calledFn := getCalledFunction(pass, e)
		if calledFn != nil {
			var fnFact FunctionErrorsFact
			if pass.ImportObjectFact(calledFn, &fnFact) {
				fact.Merge(&fnFact)
			}
		}

	case *ast.UnaryExpr:
		// Handle &MyError{}
		if e.Op.String() == "&" {
			if compLit, ok := e.X.(*ast.CompositeLit); ok {
				analyzeCompositeLit(pass, compLit, localErrs, fact, wrapped)
			}
		}

	case *ast.CompositeLit:
		// Handle MyError{}
		analyzeCompositeLit(pass, e, localErrs, fact, wrapped)
	}
}

// extractCompositeLit extracts composite literal from &MyError{} pattern.
func extractCompositeLit(call *ast.CallExpr) *ast.CompositeLit {
	// This handles cases like (&MyError{}).SomeMethod()
	// For now, we just check if the function itself is a composite lit
	if unary, ok := call.Fun.(*ast.UnaryExpr); ok {
		if unary.Op.String() == "&" {
			if compLit, ok := unary.X.(*ast.CompositeLit); ok {
				return compLit
			}
		}
	}
	return nil
}

// analyzeCompositeLit checks if a composite literal is a custom error type.
func analyzeCompositeLit(pass *analysis.Pass, compLit *ast.CompositeLit, localErrs *localErrors, fact *FunctionErrorsFact, wrapped bool) {
	// Get the type of the composite literal
	tv := pass.TypesInfo.Types[compLit]
	if !tv.IsValue() {
		return
	}

	namedType := extractNamedType(tv.Type)
	if namedType == nil {
		return
	}

	typeName := namedType.Obj()

	// Check local custom error types
	if localErrs.types[typeName] {
		fact.AddError(ErrorInfo{
			PkgPath: pass.Pkg.Path(),
			Name:    typeName.Name(),
			Wrapped: wrapped,
		})
		return
	}

	// Check imported custom error types
	var errorFact ErrorFact
	if pass.ImportObjectFact(typeName, &errorFact) {
		fact.AddError(ErrorInfo{
			PkgPath: errorFact.PkgPath,
			Name:    errorFact.Name,
			Wrapped: wrapped,
		})
	}
}

// extractNamedType extracts the named type from a type, handling pointers.
func extractNamedType(t types.Type) *types.Named {
	switch typ := t.(type) {
	case *types.Named:
		return typ
	case *types.Pointer:
		if named, ok := typ.Elem().(*types.Named); ok {
			return named
		}
	}
	return nil
}

// isFmtErrorfCall checks if the call is fmt.Errorf.
func isFmtErrorfCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	if sel.Sel.Name != "Errorf" {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	obj := pass.TypesInfo.Uses[ident]
	pkgName, ok := obj.(*types.PkgName)
	if !ok {
		return false
	}

	return pkgName.Imported().Path() == "fmt"
}

// analyzeFmtErrorfCall analyzes fmt.Errorf calls for %w wrapped errors.
func analyzeFmtErrorfCall(pass *analysis.Pass, call *ast.CallExpr, localErrs *localErrors, fact *FunctionErrorsFact) {
	if len(call.Args) < 1 {
		return
	}

	formatStr := extractStringLiteral(call.Args[0])
	if formatStr == "" {
		return
	}

	// Find %w positions in format string
	wrapIndices := findWrapVerbIndices(formatStr)
	if len(wrapIndices) == 0 {
		return
	}

	// Analyze arguments at %w positions
	for _, wrapIdx := range wrapIndices {
		argIdx := 1 + wrapIdx // args[0] is format string
		if argIdx >= len(call.Args) {
			continue
		}
		analyzeErrorExpr(pass, call.Args[argIdx], localErrs, fact, true)
	}
}

// findWrapVerbIndices finds the argument indices for %w verbs in format string.
func findWrapVerbIndices(format string) []int {
	var indices []int
	argIndex := 0

	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		if i+1 >= len(format) {
			break
		}
		i++

		// Skip flags
		for i < len(format) && strings.ContainsRune("+-# 0", rune(format[i])) {
			i++
		}

		// Skip width
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			i++
		}

		// Skip precision
		if i < len(format) && format[i] == '.' {
			i++
			for i < len(format) && format[i] >= '0' && format[i] <= '9' {
				i++
			}
		}

		if i >= len(format) {
			break
		}

		verb := format[i]
		if verb == '%' {
			continue // %% doesn't consume argument
		}

		if verb == 'w' {
			indices = append(indices, argIndex)
		}
		argIndex++
	}

	return indices
}

// getCalledFunction returns the *types.Func for a call expression if available.
func getCalledFunction(pass *analysis.Pass, call *ast.CallExpr) *types.Func {
	var obj types.Object

	switch fun := call.Fun.(type) {
	case *ast.Ident:
		obj = pass.TypesInfo.Uses[fun]
	case *ast.SelectorExpr:
		obj = pass.TypesInfo.Uses[fun.Sel]
	default:
		return nil
	}

	if fn, ok := obj.(*types.Func); ok {
		return fn
	}
	return nil
}

// analyzeClosures finds closures assigned to variables and exports facts for them.
// This handles patterns like: handler := func() error { return ErrX }
func analyzeClosures(pass *analysis.Pass, localErrs *localErrors) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
		(*ast.ValueSpec)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			analyzeClosureAssignment(pass, stmt, localErrs)
		case *ast.ValueSpec:
			analyzeClosureValueSpec(pass, stmt, localErrs)
		}
	})
}

// analyzeClosureAssignment handles: handler := func() error { ... }
func analyzeClosureAssignment(pass *analysis.Pass, stmt *ast.AssignStmt, localErrs *localErrors) {
	for i, rhs := range stmt.Rhs {
		funcLit, ok := rhs.(*ast.FuncLit)
		if !ok {
			continue
		}

		// Check if the function literal returns error
		sig := pass.TypesInfo.Types[funcLit].Type.(*types.Signature)
		errorPositions := findErrorReturnPositions(sig)
		if len(errorPositions) == 0 {
			continue
		}

		// Get the variable on the left side
		if i >= len(stmt.Lhs) {
			continue
		}
		ident, ok := stmt.Lhs[i].(*ast.Ident)
		if !ok || ident.Name == "_" {
			continue
		}

		obj := pass.TypesInfo.Defs[ident]
		if obj == nil {
			obj = pass.TypesInfo.Uses[ident]
		}
		varObj, ok := obj.(*types.Var)
		if !ok {
			continue
		}

		// Analyze the closure body for errors
		fact := &FunctionErrorsFact{}
		analyzeReturns(pass, funcLit.Body, errorPositions, localErrs, fact)

		// Export fact attached to the variable
		if len(fact.Errors) > 0 {
			pass.ExportObjectFact(varObj, fact)
		}
	}
}

// analyzeClosureValueSpec handles: var handler = func() error { ... }
func analyzeClosureValueSpec(pass *analysis.Pass, spec *ast.ValueSpec, localErrs *localErrors) {
	for i, value := range spec.Values {
		funcLit, ok := value.(*ast.FuncLit)
		if !ok {
			continue
		}

		// Check if the function literal returns error
		tv := pass.TypesInfo.Types[funcLit]
		if !tv.IsValue() {
			continue
		}
		sig, ok := tv.Type.(*types.Signature)
		if !ok {
			continue
		}
		errorPositions := findErrorReturnPositions(sig)
		if len(errorPositions) == 0 {
			continue
		}

		// Get the variable
		if i >= len(spec.Names) {
			continue
		}
		ident := spec.Names[i]
		if ident.Name == "_" {
			continue
		}

		obj := pass.TypesInfo.Defs[ident]
		varObj, ok := obj.(*types.Var)
		if !ok {
			continue
		}

		// Analyze the closure body for errors
		fact := &FunctionErrorsFact{}
		analyzeReturns(pass, funcLit.Body, errorPositions, localErrs, fact)

		// Export fact attached to the variable
		if len(fact.Errors) > 0 {
			pass.ExportObjectFact(varObj, fact)
		}
	}
}

// computeInterfaceMethodFacts computes and exports InterfaceMethodFact for each
// interface method in the package. It collects errors from all known implementations.
func computeInterfaceMethodFacts(pass *analysis.Pass, localFacts map[*types.Func]*FunctionErrorsFact, impls *interfaceImplementations) {
	scope := pass.Pkg.Scope()

	// For each interface defined in this package
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		typeName, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}

		ifaceType, ok := typeName.Type().Underlying().(*types.Interface)
		if !ok {
			continue
		}

		// For each method in the interface
		for i := 0; i < ifaceType.NumMethods(); i++ {
			ifaceMethod := ifaceType.Method(i)

			// Skip methods from embedded interfaces (they belong to other packages)
			if ifaceMethod.Pkg() != pass.Pkg {
				continue
			}

			fact := &InterfaceMethodFact{}

			// Collect errors from all implementations
			implementingTypes := impls.getImplementingTypes(ifaceType)
			for _, concreteType := range implementingTypes {
				method := findMethodImplementation(concreteType, ifaceMethod)
				if method == nil {
					continue
				}

				// Check local facts first
				if localFact, ok := localFacts[method]; ok {
					fact.AddErrors(localFact.Errors)
				}

				// Also check imported facts
				var fnFact FunctionErrorsFact
				if pass.ImportObjectFact(method, &fnFact) {
					fact.AddErrors(fnFact.Errors)
				}
			}

			// Export the fact if we found any errors
			if len(fact.Errors) > 0 {
				pass.ExportObjectFact(ifaceMethod, fact)
			}
		}
	}
}
