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
// FunctionSentinelsFact for each function that can return sentinel errors.
// It uses iterative analysis to handle factory functions that call other
// functions returning sentinels.
func analyzeFunctionReturns(pass *analysis.Pass, sentinels *localSentinels) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

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
	localFacts := make(map[*types.Func]*FunctionSentinelsFact)

	// Iterate until no new facts are discovered
	for {
		changed := false

		for _, fi := range funcs {
			fact := &FunctionSentinelsFact{}
			analyzeReturnsWithLocalFacts(pass, fi.body, fi.errorPositions, sentinels, fact, localFacts)

			existing := localFacts[fi.fn]
			if existing == nil {
				if len(fact.Sentinels) > 0 {
					localFacts[fi.fn] = fact
					changed = true
				}
			} else {
				// Check if we found new sentinels
				oldLen := len(existing.Sentinels)
				existing.Merge(fact)
				if len(existing.Sentinels) > oldLen {
					changed = true
				}
			}
		}

		if !changed {
			break
		}
	}

	// Export all discovered facts
	for fn, fact := range localFacts {
		if len(fact.Sentinels) > 0 {
			pass.ExportObjectFact(fn, fact)
		}
	}
}

// analyzeReturnsWithLocalFacts is like analyzeReturns but also checks local function facts.
func analyzeReturnsWithLocalFacts(pass *analysis.Pass, body *ast.BlockStmt, errorPositions []int, sentinels *localSentinels, fact *FunctionSentinelsFact, localFacts map[*types.Func]*FunctionSentinelsFact) {
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		for _, pos := range errorPositions {
			if pos < len(ret.Results) {
				analyzeSentinelExprWithLocalFacts(pass, ret.Results[pos], sentinels, fact, false, localFacts)
			}
		}

		return true
	})
}

// analyzeSentinelExprWithLocalFacts is like analyzeSentinelExpr but also checks local function facts.
func analyzeSentinelExprWithLocalFacts(pass *analysis.Pass, expr ast.Expr, sentinels *localSentinels, fact *FunctionSentinelsFact, wrapped bool, localFacts map[*types.Func]*FunctionSentinelsFact) {
	switch e := expr.(type) {
	case *ast.Ident:
		obj := pass.TypesInfo.Uses[e]
		if obj == nil {
			return
		}

		if varObj, ok := obj.(*types.Var); ok {
			if sentinels.vars[varObj] {
				fact.AddSentinel(SentinelInfo{
					PkgPath: pass.Pkg.Path(),
					Name:    varObj.Name(),
					Wrapped: wrapped,
				})
				return
			}
			var sentinelFact SentinelErrorFact
			if pass.ImportObjectFact(varObj, &sentinelFact) {
				fact.AddSentinel(SentinelInfo{
					PkgPath: sentinelFact.PkgPath,
					Name:    sentinelFact.Name,
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
			var sentinelFact SentinelErrorFact
			if pass.ImportObjectFact(varObj, &sentinelFact) {
				fact.AddSentinel(SentinelInfo{
					PkgPath: sentinelFact.PkgPath,
					Name:    sentinelFact.Name,
					Wrapped: wrapped,
				})
			}
		}

	case *ast.CallExpr:
		if isFmtErrorfCall(pass, e) {
			analyzeFmtErrorfCallWithLocalFacts(pass, e, sentinels, fact, localFacts)
			return
		}

		if compLit := extractCompositeLit(e); compLit != nil {
			analyzeCompositeLit(pass, compLit, sentinels, fact, wrapped)
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
			var fnFact FunctionSentinelsFact
			if pass.ImportObjectFact(calledFn, &fnFact) {
				fact.Merge(&fnFact)
			}
		}

	case *ast.UnaryExpr:
		if e.Op.String() == "&" {
			if compLit, ok := e.X.(*ast.CompositeLit); ok {
				analyzeCompositeLit(pass, compLit, sentinels, fact, wrapped)
			}
		}

	case *ast.CompositeLit:
		analyzeCompositeLit(pass, e, sentinels, fact, wrapped)
	}
}

// analyzeFmtErrorfCallWithLocalFacts is like analyzeFmtErrorfCall but uses local facts.
func analyzeFmtErrorfCallWithLocalFacts(pass *analysis.Pass, call *ast.CallExpr, sentinels *localSentinels, fact *FunctionSentinelsFact, localFacts map[*types.Func]*FunctionSentinelsFact) {
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
		analyzeSentinelExprWithLocalFacts(pass, call.Args[argIdx], sentinels, fact, true, localFacts)
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
func analyzeReturns(pass *analysis.Pass, body *ast.BlockStmt, errorPositions []int, sentinels *localSentinels, fact *FunctionSentinelsFact) {
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		// Analyze each error return position
		for _, pos := range errorPositions {
			if pos < len(ret.Results) {
				analyzeSentinelExpr(pass, ret.Results[pos], sentinels, fact, false)
			}
		}

		return true
	})
}

// analyzeSentinelExpr analyzes an expression to find sentinel errors.
func analyzeSentinelExpr(pass *analysis.Pass, expr ast.Expr, sentinels *localSentinels, fact *FunctionSentinelsFact, wrapped bool) {
	switch e := expr.(type) {
	case *ast.Ident:
		// Direct sentinel return: return ErrNotFound
		obj := pass.TypesInfo.Uses[e]
		if obj == nil {
			return
		}

		if varObj, ok := obj.(*types.Var); ok {
			// Check local sentinels
			if sentinels.vars[varObj] {
				fact.AddSentinel(SentinelInfo{
					PkgPath: pass.Pkg.Path(),
					Name:    varObj.Name(),
					Wrapped: wrapped,
				})
				return
			}
			// Check imported sentinel via fact
			var sentinelFact SentinelErrorFact
			if pass.ImportObjectFact(varObj, &sentinelFact) {
				fact.AddSentinel(SentinelInfo{
					PkgPath: sentinelFact.PkgPath,
					Name:    sentinelFact.Name,
					Wrapped: wrapped,
				})
			}
		}

	case *ast.SelectorExpr:
		// Qualified sentinel: return pkg.ErrNotFound
		obj := pass.TypesInfo.Uses[e.Sel]
		if obj == nil {
			return
		}

		if varObj, ok := obj.(*types.Var); ok {
			var sentinelFact SentinelErrorFact
			if pass.ImportObjectFact(varObj, &sentinelFact) {
				fact.AddSentinel(SentinelInfo{
					PkgPath: sentinelFact.PkgPath,
					Name:    sentinelFact.Name,
					Wrapped: wrapped,
				})
			}
		}

	case *ast.CallExpr:
		// Check for fmt.Errorf with %w wrapping a sentinel
		if isFmtErrorfCall(pass, e) {
			analyzeFmtErrorfCall(pass, e, sentinels, fact)
			return
		}

		// Check for custom error type construction: &MyError{} or MyError{}
		if compLit := extractCompositeLit(e); compLit != nil {
			analyzeCompositeLit(pass, compLit, sentinels, fact, wrapped)
			return
		}

		// Check for function calls that might return sentinels
		calledFn := getCalledFunction(pass, e)
		if calledFn != nil {
			var fnFact FunctionSentinelsFact
			if pass.ImportObjectFact(calledFn, &fnFact) {
				fact.Merge(&fnFact)
			}
		}

	case *ast.UnaryExpr:
		// Handle &MyError{}
		if e.Op.String() == "&" {
			if compLit, ok := e.X.(*ast.CompositeLit); ok {
				analyzeCompositeLit(pass, compLit, sentinels, fact, wrapped)
			}
		}

	case *ast.CompositeLit:
		// Handle MyError{}
		analyzeCompositeLit(pass, e, sentinels, fact, wrapped)
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
func analyzeCompositeLit(pass *analysis.Pass, compLit *ast.CompositeLit, sentinels *localSentinels, fact *FunctionSentinelsFact, wrapped bool) {
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
	if sentinels.types[typeName] {
		fact.AddSentinel(SentinelInfo{
			PkgPath: pass.Pkg.Path(),
			Name:    typeName.Name(),
			Wrapped: wrapped,
		})
		return
	}

	// Check imported custom error types
	var sentinelFact SentinelErrorFact
	if pass.ImportObjectFact(typeName, &sentinelFact) {
		fact.AddSentinel(SentinelInfo{
			PkgPath: sentinelFact.PkgPath,
			Name:    sentinelFact.Name,
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

// analyzeFmtErrorfCall analyzes fmt.Errorf calls for %w wrapped sentinels.
func analyzeFmtErrorfCall(pass *analysis.Pass, call *ast.CallExpr, sentinels *localSentinels, fact *FunctionSentinelsFact) {
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
		analyzeSentinelExpr(pass, call.Args[argIdx], sentinels, fact, true)
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
func analyzeClosures(pass *analysis.Pass, sentinels *localSentinels) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.AssignStmt)(nil),
		(*ast.ValueSpec)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			analyzeClosureAssignment(pass, stmt, sentinels)
		case *ast.ValueSpec:
			analyzeClosureValueSpec(pass, stmt, sentinels)
		}
	})
}

// analyzeClosureAssignment handles: handler := func() error { ... }
func analyzeClosureAssignment(pass *analysis.Pass, stmt *ast.AssignStmt, sentinels *localSentinels) {
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

		// Analyze the closure body for sentinels
		fact := &FunctionSentinelsFact{}
		analyzeReturns(pass, funcLit.Body, errorPositions, sentinels, fact)

		// Export fact attached to the variable
		if len(fact.Sentinels) > 0 {
			pass.ExportObjectFact(varObj, fact)
		}
	}
}

// analyzeClosureValueSpec handles: var handler = func() error { ... }
func analyzeClosureValueSpec(pass *analysis.Pass, spec *ast.ValueSpec, sentinels *localSentinels) {
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

		// Analyze the closure body for sentinels
		fact := &FunctionSentinelsFact{}
		analyzeReturns(pass, funcLit.Body, errorPositions, sentinels, fact)

		// Export fact attached to the variable
		if len(fact.Sentinels) > 0 {
			pass.ExportObjectFact(varObj, fact)
		}
	}
}
