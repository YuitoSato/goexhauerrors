package analyzer

import (
	"go/ast"
	"go/types"

	"github.com/YuitoSato/goexhauerrors/goexhauerrors/detector"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/facts"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/internal"
	ssaanalysis "github.com/YuitoSato/goexhauerrors/goexhauerrors/ssa"
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

// AnalyzeFunctionReturns analyzes all functions in the package and exports
// FunctionErrorsFact for each function that can return errors.
// It uses iterative analysis to handle factory functions that call other
// functions returning errors, combined with SSA-based analysis to track
// errors through variables.
func AnalyzeFunctionReturns(pass *analysis.Pass, localErrs *detector.LocalErrors) (map[*types.Func]*facts.FunctionErrorsFact, map[*types.Func]*facts.ParameterFlowFact, map[*types.Func]*facts.FunctionParamCallFlowFact, *internal.InterfaceImplementations) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Discover interface implementations in this package
	interfaceImpls := internal.FindInterfaceImplementations(pass)

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
		errorPositions := internal.FindErrorReturnPositions(sig)
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
	localFacts := make(map[*types.Func]*facts.FunctionErrorsFact)
	localParamFlowFacts := make(map[*types.Func]*facts.ParameterFlowFact)
	localCallFlowFacts := make(map[*types.Func]*facts.FunctionParamCallFlowFact)

	// Iterate until no new facts are discovered (AST-based + SSA-based analysis)
	for {
		changed := false

		// Create SSA analyzer with current local facts
		ssaAnalyzer := ssaanalysis.NewAnalyzer(pass, localErrs, localFacts, localParamFlowFacts, localCallFlowFacts, interfaceImpls)

		for _, fi := range funcs {
			// Phase A: Detect parameter flow (for error-typed parameters)
			paramFlowFact := ssaAnalyzer.DetectParameterFlow(fi.fn, fi.errorPositions)
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
			callFlowFact := ssaAnalyzer.DetectFunctionParamCallFlow(fi.fn, fi.errorPositions)
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
			fact := &facts.FunctionErrorsFact{}

			// AST-based analysis
			analyzeReturnsWithLocalFacts(pass, fi.body, fi.errorPositions, localErrs, fact, localFacts)

			// SSA-based analysis to trace errors through variables
			ssaErrors := ssaAnalyzer.TraceReturnStatements(fi.fn, fi.errorPositions)
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

	// Return data needed for interface method facts (computed after AnalyzeParameterErrorChecks)
	return localFacts, localParamFlowFacts, localCallFlowFacts, interfaceImpls
}

// buildValidErrors creates a set of valid error keys that can be used in FunctionErrorsFact.
// A error is valid if:
// 1. It's a local error (var or type) in the current package
// 2. It has an imported ErrorFact (excluding ignored packages)
func buildValidErrors(pass *analysis.Pass, localErrs *detector.LocalErrors) map[string]bool {
	valid := make(map[string]bool)

	// Add local error variables
	for varObj := range localErrs.Vars {
		key := pass.Pkg.Path() + "." + varObj.Name()
		valid[key] = true
	}

	// Add local custom error types
	for typeName := range localErrs.Types {
		key := pass.Pkg.Path() + "." + typeName.Name()
		valid[key] = true
	}

	// Note: Imported errors are validated by checking ImportObjectFact during analysis
	// We need to also allow imported errors that have facts
	// This is done by scanning all referenced packages for exported facts
	for _, imp := range pass.Pkg.Imports() {
		// Skip ignored packages
		if internal.ShouldIgnorePackage(imp.Path()) {
			continue
		}

		scope := imp.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			// Check for error variable facts
			if varObj, ok := obj.(*types.Var); ok {
				var errorFact facts.ErrorFact
				if pass.ImportObjectFact(varObj, &errorFact) {
					valid[errorFact.Key()] = true
				}
			}
			// Check for error type facts
			if typeName, ok := obj.(*types.TypeName); ok {
				var errorFact facts.ErrorFact
				if pass.ImportObjectFact(typeName, &errorFact) {
					valid[errorFact.Key()] = true
				}
			}
		}
	}

	return valid
}

// analyzeReturnsWithLocalFacts is like analyzeReturns but also checks local function facts.
func analyzeReturnsWithLocalFacts(pass *analysis.Pass, body *ast.BlockStmt, errorPositions []int, localErrs *detector.LocalErrors, fact *facts.FunctionErrorsFact, localFacts map[*types.Func]*facts.FunctionErrorsFact) {
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
func analyzeErrorExprWithLocalFacts(pass *analysis.Pass, expr ast.Expr, localErrs *detector.LocalErrors, fact *facts.FunctionErrorsFact, wrapped bool, localFacts map[*types.Func]*facts.FunctionErrorsFact) {
	switch e := expr.(type) {
	case *ast.Ident:
		obj := pass.TypesInfo.Uses[e]
		if obj == nil {
			return
		}

		if varObj, ok := obj.(*types.Var); ok {
			if localErrs.Vars[varObj] {
				fact.AddError(facts.ErrorInfo{
					PkgPath: pass.Pkg.Path(),
					Name:    varObj.Name(),
					Wrapped: wrapped,
				})
				return
			}
			var errorFact facts.ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				fact.AddError(facts.ErrorInfo{
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
			var errorFact facts.ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				fact.AddError(facts.ErrorInfo{
					PkgPath: errorFact.PkgPath,
					Name:    errorFact.Name,
					Wrapped: wrapped,
				})
			}
		}

	case *ast.CallExpr:
		if internal.IsFmtErrorfCall(pass, e) {
			analyzeFmtErrorfCallWithLocalFacts(pass, e, localErrs, fact, localFacts)
			return
		}

		if compLit := internal.ExtractCompositeLit(e); compLit != nil {
			analyzeCompositeLit(pass, compLit, localErrs, fact, wrapped)
			return
		}

		// Check for function calls - first check local facts, then imported facts
		calledFn := internal.GetCalledFunction(pass, e)
		if calledFn != nil {
			// Check local facts first (for same-package functions)
			if localFact, ok := localFacts[calledFn]; ok {
				fact.Merge(localFact)
			}
			// Also check imported facts (for cross-package or already exported)
			var fnFact facts.FunctionErrorsFact
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
func analyzeFmtErrorfCallWithLocalFacts(pass *analysis.Pass, call *ast.CallExpr, localErrs *detector.LocalErrors, fact *facts.FunctionErrorsFact, localFacts map[*types.Func]*facts.FunctionErrorsFact) {
	if len(call.Args) < 1 {
		return
	}

	formatStr := internal.ExtractStringLiteral(call.Args[0])
	if formatStr == "" {
		return
	}

	wrapIndices := internal.FindWrapVerbIndices(formatStr)
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

// analyzeReturns walks through the function body and analyzes return statements.
func analyzeReturns(pass *analysis.Pass, body *ast.BlockStmt, errorPositions []int, localErrs *detector.LocalErrors, fact *facts.FunctionErrorsFact) {
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
func analyzeErrorExpr(pass *analysis.Pass, expr ast.Expr, localErrs *detector.LocalErrors, fact *facts.FunctionErrorsFact, wrapped bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		// Direct error return: return ErrNotFound
		obj := pass.TypesInfo.Uses[e]
		if obj == nil {
			return
		}

		if varObj, ok := obj.(*types.Var); ok {
			// Check local errors
			if localErrs.Vars[varObj] {
				fact.AddError(facts.ErrorInfo{
					PkgPath: pass.Pkg.Path(),
					Name:    varObj.Name(),
					Wrapped: wrapped,
				})
				return
			}
			// Check imported error via fact
			var errorFact facts.ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				fact.AddError(facts.ErrorInfo{
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
			var errorFact facts.ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				fact.AddError(facts.ErrorInfo{
					PkgPath: errorFact.PkgPath,
					Name:    errorFact.Name,
					Wrapped: wrapped,
				})
			}
		}

	case *ast.CallExpr:
		// Check for fmt.Errorf with %w wrapping a error
		if internal.IsFmtErrorfCall(pass, e) {
			analyzeFmtErrorfCall(pass, e, localErrs, fact)
			return
		}

		// Check for custom error type construction: &MyError{} or MyError{}
		if compLit := internal.ExtractCompositeLit(e); compLit != nil {
			analyzeCompositeLit(pass, compLit, localErrs, fact, wrapped)
			return
		}

		// Check for function calls that might return errors
		calledFn := internal.GetCalledFunction(pass, e)
		if calledFn != nil {
			var fnFact facts.FunctionErrorsFact
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

// analyzeCompositeLit checks if a composite literal is a custom error type.
func analyzeCompositeLit(pass *analysis.Pass, compLit *ast.CompositeLit, localErrs *detector.LocalErrors, fact *facts.FunctionErrorsFact, wrapped bool) {
	// Get the type of the composite literal
	tv := pass.TypesInfo.Types[compLit]
	if !tv.IsValue() {
		return
	}

	namedType := internal.ExtractNamedType(tv.Type)
	if namedType == nil {
		return
	}

	typeName := namedType.Obj()

	// Check local custom error types
	if localErrs.Types[typeName] {
		fact.AddError(facts.ErrorInfo{
			PkgPath: pass.Pkg.Path(),
			Name:    typeName.Name(),
			Wrapped: wrapped,
		})
		return
	}

	// Check imported custom error types
	var errorFact facts.ErrorFact
	if pass.ImportObjectFact(typeName, &errorFact) {
		fact.AddError(facts.ErrorInfo{
			PkgPath: errorFact.PkgPath,
			Name:    errorFact.Name,
			Wrapped: wrapped,
		})
	}
}

// analyzeFmtErrorfCall analyzes fmt.Errorf calls for %w wrapped errors.
func analyzeFmtErrorfCall(pass *analysis.Pass, call *ast.CallExpr, localErrs *detector.LocalErrors, fact *facts.FunctionErrorsFact) {
	if len(call.Args) < 1 {
		return
	}

	formatStr := internal.ExtractStringLiteral(call.Args[0])
	if formatStr == "" {
		return
	}

	// Find %w positions in format string
	wrapIndices := internal.FindWrapVerbIndices(formatStr)
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

// AnalyzeClosures finds closures assigned to variables and exports facts for them.
// This handles patterns like: handler := func() error { return ErrX }
func AnalyzeClosures(pass *analysis.Pass, localErrs *detector.LocalErrors) {
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
func analyzeClosureAssignment(pass *analysis.Pass, stmt *ast.AssignStmt, localErrs *detector.LocalErrors) {
	for i, rhs := range stmt.Rhs {
		funcLit, ok := rhs.(*ast.FuncLit)
		if !ok {
			continue
		}

		// Check if the function literal returns error
		sig := pass.TypesInfo.Types[funcLit].Type.(*types.Signature)
		errorPositions := internal.FindErrorReturnPositions(sig)
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
		fact := &facts.FunctionErrorsFact{}
		analyzeReturns(pass, funcLit.Body, errorPositions, localErrs, fact)

		// Export fact attached to the variable
		if len(fact.Errors) > 0 {
			pass.ExportObjectFact(varObj, fact)
		}
	}
}

// analyzeClosureValueSpec handles: var handler = func() error { ... }
func analyzeClosureValueSpec(pass *analysis.Pass, spec *ast.ValueSpec, localErrs *detector.LocalErrors) {
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
		errorPositions := internal.FindErrorReturnPositions(sig)
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
		fact := &facts.FunctionErrorsFact{}
		analyzeReturns(pass, funcLit.Body, errorPositions, localErrs, fact)

		// Export fact attached to the variable
		if len(fact.Errors) > 0 {
			pass.ExportObjectFact(varObj, fact)
		}
	}
}

// ComputeInterfaceMethodFacts computes and exports InterfaceMethodFact, ParameterFlowFact,
// FunctionParamCallFlowFact, and ParameterCheckedErrorsFact for each interface method in the package.
// It collects errors from all known implementations.
// For ParameterFlowFact, FunctionParamCallFlowFact, and ParameterCheckedErrorsFact,
// intersection semantics is used (a fact is only exported if ALL implementations agree).
func ComputeInterfaceMethodFacts(pass *analysis.Pass, localFacts map[*types.Func]*facts.FunctionErrorsFact, localParamFlowFacts map[*types.Func]*facts.ParameterFlowFact, localCallFlowFacts map[*types.Func]*facts.FunctionParamCallFlowFact, impls *internal.InterfaceImplementations) {
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

			fact := &facts.InterfaceMethodFact{}
			var allParamFlowFacts []*facts.ParameterFlowFact
			var allCallFlowFacts []*facts.FunctionParamCallFlowFact
			var allCheckedFacts []*facts.ParameterCheckedErrorsFact

			// Collect facts from all implementations
			implementingTypes := impls.GetImplementingTypes(ifaceType)
			for _, concreteType := range implementingTypes {
				method := internal.FindMethodImplementation(concreteType, ifaceMethod)
				if method == nil {
					continue
				}

				// FunctionErrorsFact (union)
				if localFact, ok := localFacts[method]; ok {
					fact.AddErrors(localFact.Errors)
				}
				var fnFact facts.FunctionErrorsFact
				if pass.ImportObjectFact(method, &fnFact) {
					fact.AddErrors(fnFact.Errors)
				}

				// ParameterFlowFact (collect for intersection)
				var pf *facts.ParameterFlowFact
				if localPF, ok := localParamFlowFacts[method]; ok {
					pf = localPF
				}
				var importedPF facts.ParameterFlowFact
				if pass.ImportObjectFact(method, &importedPF) {
					if pf == nil {
						pf = &importedPF
					} else {
						pf.Merge(&importedPF)
					}
				}
				allParamFlowFacts = append(allParamFlowFacts, pf)

				// FunctionParamCallFlowFact (collect for intersection)
				var pcf *facts.FunctionParamCallFlowFact
				if localCF, ok := localCallFlowFacts[method]; ok {
					pcf = localCF
				}
				var importedPCF facts.FunctionParamCallFlowFact
				if pass.ImportObjectFact(method, &importedPCF) {
					if pcf == nil {
						pcf = &importedPCF
					} else {
						pcf.Merge(&importedPCF)
					}
				}
				allCallFlowFacts = append(allCallFlowFacts, pcf)

				// ParameterCheckedErrorsFact (collect for intersection)
				var cf *facts.ParameterCheckedErrorsFact
				var importedCF facts.ParameterCheckedErrorsFact
				if pass.ImportObjectFact(method, &importedCF) {
					cf = &importedCF
				}
				allCheckedFacts = append(allCheckedFacts, cf)
			}

			// Export InterfaceMethodFact (union of errors)
			if len(fact.Errors) > 0 {
				pass.ExportObjectFact(ifaceMethod, fact)
			}

			// Export ParameterFlowFact (intersection across implementations)
			if len(allParamFlowFacts) > 0 {
				intersectedFlow := facts.IntersectParameterFlowFacts(allParamFlowFacts)
				if intersectedFlow != nil && len(intersectedFlow.Flows) > 0 {
					pass.ExportObjectFact(ifaceMethod, intersectedFlow)
				}
			}

			// Export FunctionParamCallFlowFact (intersection across implementations)
			if len(allCallFlowFacts) > 0 {
				intersectedCallFlow := facts.IntersectFunctionParamCallFlowFacts(allCallFlowFacts)
				if intersectedCallFlow != nil && len(intersectedCallFlow.CallFlows) > 0 {
					pass.ExportObjectFact(ifaceMethod, intersectedCallFlow)
				}
			}

			// Export ParameterCheckedErrorsFact (intersection across implementations)
			if len(allCheckedFacts) > 0 {
				intersectedChecked := facts.IntersectParameterCheckedErrorsFacts(allCheckedFacts)
				if intersectedChecked != nil && len(intersectedChecked.Checks) > 0 {
					pass.ExportObjectFact(ifaceMethod, intersectedChecked)
				}
			}
		}
	}
}

// ComputeImportedInterfaceMethodFacts computes InterfaceMethodFact and
// FunctionParamCallFlowFact for interfaces defined in imported packages,
// using implementations found in the current package.
// This handles the DI pattern where the interface is in package A (e.g., domain),
// the implementation is in package B (e.g., infra), and the caller is in package C
// (e.g., usecase) which imports A but not B. By storing facts in a global store
// from B, C can later look them up even without importing B.
func ComputeImportedInterfaceMethodFacts(pass *analysis.Pass, localFacts map[*types.Func]*facts.FunctionErrorsFact, localCallFlowFacts map[*types.Func]*facts.FunctionParamCallFlowFact, impls *internal.InterfaceImplementations) {
	for _, imp := range pass.Pkg.Imports() {
		impScope := imp.Scope()
		for _, name := range impScope.Names() {
			obj := impScope.Lookup(name)
			typeName, ok := obj.(*types.TypeName)
			if !ok {
				continue
			}

			ifaceType, ok := typeName.Type().Underlying().(*types.Interface)
			if !ok {
				continue
			}

			// Find implementations of this imported interface in the current package
			implementingTypes := impls.GetImplementingTypes(ifaceType)
			if len(implementingTypes) == 0 {
				continue
			}

			for i := 0; i < ifaceType.NumMethods(); i++ {
				ifaceMethod := ifaceType.Method(i)

				fact := &facts.InterfaceMethodFact{}
				var allCallFlowFacts []*facts.FunctionParamCallFlowFact

				for _, concreteType := range implementingTypes {
					method := internal.FindMethodImplementation(concreteType, ifaceMethod)
					if method == nil {
						continue
					}

					// Collect errors from local implementations (union)
					if localFact, ok := localFacts[method]; ok {
						fact.AddErrors(localFact.Errors)
					}
					var fnFact facts.FunctionErrorsFact
					if pass.ImportObjectFact(method, &fnFact) {
						fact.AddErrors(fnFact.Errors)
					}

					// Collect FunctionParamCallFlowFact (for intersection)
					var pcf *facts.FunctionParamCallFlowFact
					if localCF, ok := localCallFlowFacts[method]; ok {
						pcf = localCF
					}
					var importedCF facts.FunctionParamCallFlowFact
					if pass.ImportObjectFact(method, &importedCF) {
						if pcf == nil {
							pcf = &importedCF
						} else {
							pcf.Merge(&importedCF)
						}
					}
					allCallFlowFacts = append(allCallFlowFacts, pcf)
				}

				key := facts.InterfaceMethodKey(imp.Path(), typeName.Name(), ifaceMethod.Name())

				if len(fact.Errors) > 0 {
					// Store in global store so callers that don't import this package can find it
					facts.MergeInterfaceMethodFact(key, fact)
				}

				// Store FunctionParamCallFlowFact in global store (intersection semantics)
				if len(allCallFlowFacts) > 0 {
					intersected := facts.IntersectFunctionParamCallFlowFacts(allCallFlowFacts)
					facts.MergeCallFlowFact(key, intersected)
				}
			}
		}
	}
}

// AnalyzeParameterErrorChecks analyzes all functions to detect errors.Is/As checks
// performed on error-typed parameters inside the function body.
// This allows callers to know which errors are already checked inside the function.
func AnalyzeParameterErrorChecks(pass *analysis.Pass) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

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

		// Find error-typed parameters
		errorParams := internal.FindErrorParamVars(sig)
		if len(errorParams) == 0 {
			return
		}

		// Analyze the body for errors.Is/As calls on those parameters
		fact := detectParameterErrorChecks(pass, funcDecl.Body, errorParams)
		if fact != nil && len(fact.Checks) > 0 {
			pass.ExportObjectFact(fn, fact)
		}
	})
}

// detectParameterErrorChecks walks a function body and detects errors.Is/As calls
// on the given error parameters.
func detectParameterErrorChecks(pass *analysis.Pass, body *ast.BlockStmt, errorParams map[*types.Var]int) *facts.ParameterCheckedErrorsFact {
	fact := &facts.ParameterCheckedErrorsFact{}

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if internal.IsErrorsIsCall(pass, call) && len(call.Args) >= 2 {
			for paramVar, paramIdx := range errorParams {
				if internal.ReferencesVariable(pass, call.Args[0], paramVar) {
					errInfo := internal.ExtractErrorInfoFromExpr(pass, call.Args[1])
					if errInfo != nil {
						fact.AddCheck(paramIdx, *errInfo)
					}
				}
			}
		}

		if internal.IsErrorsAsCall(pass, call) && len(call.Args) >= 2 {
			for paramVar, paramIdx := range errorParams {
				if internal.ReferencesVariable(pass, call.Args[0], paramVar) {
					errInfo := internal.ExtractErrorInfoFromAsTarget(pass, call.Args[1])
					if errInfo != nil {
						fact.AddCheck(paramIdx, *errInfo)
					}
				}
			}
		}

		return true
	})

	if len(fact.Checks) == 0 {
		return nil
	}
	return fact
}
