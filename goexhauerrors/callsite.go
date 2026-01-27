package goexhauerrors

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// checkCallSites checks all call sites to ensure errors are properly checked.
func checkCallSites(pass *analysis.Pass) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		funcDecl := n.(*ast.FuncDecl)
		if funcDecl.Body == nil {
			return
		}

		// Check if this function returns error (for propagation detection)
		returnsError := funcReturnsError(pass, funcDecl)

		// Find all call expressions and their error assignments
		checkFunctionBody(pass, funcDecl.Body, returnsError)
	})
}

// funcReturnsError checks if the function returns an error type.
func funcReturnsError(pass *analysis.Pass, funcDecl *ast.FuncDecl) bool {
	funcObj := pass.TypesInfo.Defs[funcDecl.Name]
	if funcObj == nil {
		return false
	}
	fn, ok := funcObj.(*types.Func)
	if !ok {
		return false
	}
	sig := fn.Type().(*types.Signature)
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		if isErrorType(results.At(i).Type()) {
			return true
		}
	}
	return false
}

// errorVarState tracks the active errors for an error variable.
type errorVarState struct {
	callPos token.Pos
	errors  []ErrorInfo
	checked map[string]bool
}

// checkFunctionBody checks all call sites within a function body using flow-sensitive analysis.
func checkFunctionBody(pass *analysis.Pass, body *ast.BlockStmt, canPropagate bool) {
	// Track active error states for each variable
	states := make(map[*types.Var]*errorVarState)

	// Walk statements in order for flow-sensitive analysis
	walkStatementsWithScope(pass, body.List, states, canPropagate)

	// Report any remaining unchecked errors at end of function
	for _, state := range states {
		reportUncheckedErrors(pass, state)
	}
}

// walkStatementsWithScope walks statements and tracks error variable states.
func walkStatementsWithScope(pass *analysis.Pass, stmts []ast.Stmt, states map[*types.Var]*errorVarState, canPropagate bool) {
	for _, stmt := range stmts {
		walkStatementWithScope(pass, stmt, states, canPropagate)
	}
}

// walkStatementWithScope processes a single statement for error tracking.
func walkStatementWithScope(pass *analysis.Pass, stmt ast.Stmt, states map[*types.Var]*errorVarState, canPropagate bool) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		// First, check for errors.Is in RHS expressions (before assignment)
		for _, rhs := range s.Rhs {
			collectErrorsIsInExpr(pass, rhs, states)
		}

		// Then process assignments
		for i, rhs := range s.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok {
				continue
			}

			fnFact, sig := getCallErrors(pass, call)
			if fnFact == nil || len(fnFact.Errors) == 0 {
				continue
			}

			errorVar := findErrorVarInAssignmentWithSig(pass, s, i, sig)
			if errorVar == nil {
				continue
			}

			// If this variable already has active errors, report unchecked ones
			if existingState, ok := states[errorVar]; ok {
				reportUncheckedErrors(pass, existingState)
			}

			// Set new state for this variable
			states[errorVar] = &errorVarState{
				callPos:   call.Pos(),
				errors: fnFact.Errors,
				checked:   make(map[string]bool),
			}
		}

	case *ast.ExprStmt:
		// Check for errors.Is calls in expression statements
		collectErrorsIsInExpr(pass, s.X, states)

	case *ast.IfStmt:
		// Check condition for errors.Is
		collectErrorsIsInExpr(pass, s.Cond, states)

		// Process init statement if present
		if s.Init != nil {
			walkStatementWithScope(pass, s.Init, states, canPropagate)
		}

		// Clone states for branches
		ifStates := cloneStates(states)
		elseStates := cloneStates(states)

		// Process if body
		walkStatementsWithScope(pass, s.Body.List, ifStates, canPropagate)

		// Process else body if present
		if s.Else != nil {
			switch elseStmt := s.Else.(type) {
			case *ast.BlockStmt:
				walkStatementsWithScope(pass, elseStmt.List, elseStates, canPropagate)
			case *ast.IfStmt:
				walkStatementWithScope(pass, elseStmt, elseStates, canPropagate)
			}
		}

		// Merge states: mark errors as checked if checked in BOTH branches
		mergeStates(states, ifStates, elseStates)

	case *ast.SwitchStmt:
		if s.Init != nil {
			walkStatementWithScope(pass, s.Init, states, canPropagate)
		}
		if s.Tag != nil {
			collectErrorsIsInExpr(pass, s.Tag, states)
		}
		// Process switch body
		if s.Body != nil {
			for _, clause := range s.Body.List {
				if cc, ok := clause.(*ast.CaseClause); ok {
					// Check case expressions for errors.Is
					for _, expr := range cc.List {
						collectErrorsIsInExpr(pass, expr, states)
					}
					// Walk case body
					caseStates := cloneStates(states)
					walkStatementsWithScope(pass, cc.Body, caseStates, canPropagate)
					// Merge back checked errors
					for varObj, caseState := range caseStates {
						if state, ok := states[varObj]; ok {
							for key := range caseState.checked {
								state.checked[key] = true
							}
						}
					}
				}
			}
		}

	case *ast.ReturnStmt:
		// Check if error variables are propagated
		for varObj, state := range states {
			for _, result := range s.Results {
				if referencesVariable(pass, result, varObj) {
					if canPropagate {
						// Mark all errors as "checked" since we're propagating
						for _, errInfo := range state.errors {
							state.checked[errInfo.Key()] = true
						}
					}
				}
			}
		}

	case *ast.BlockStmt:
		walkStatementsWithScope(pass, s.List, states, canPropagate)

	case *ast.ForStmt:
		if s.Init != nil {
			walkStatementWithScope(pass, s.Init, states, canPropagate)
		}
		if s.Cond != nil {
			collectErrorsIsInExpr(pass, s.Cond, states)
		}
		if s.Post != nil {
			walkStatementWithScope(pass, s.Post, states, canPropagate)
		}
		if s.Body != nil {
			walkStatementsWithScope(pass, s.Body.List, states, canPropagate)
		}

	case *ast.RangeStmt:
		if s.Body != nil {
			walkStatementsWithScope(pass, s.Body.List, states, canPropagate)
		}

	case *ast.DeclStmt:
		if genDecl, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range genDecl.Specs {
				if valueSpec, ok := spec.(*ast.ValueSpec); ok {
					for i, value := range valueSpec.Values {
						call, ok := value.(*ast.CallExpr)
						if !ok {
							continue
						}

						fnFact, _ := getCallErrors(pass, call)
						if fnFact == nil || len(fnFact.Errors) == 0 {
							continue
						}

						if i < len(valueSpec.Names) {
							obj := pass.TypesInfo.Defs[valueSpec.Names[i]]
							if varObj, ok := obj.(*types.Var); ok {
								if isErrorType(varObj.Type()) {
									states[varObj] = &errorVarState{
										callPos:   call.Pos(),
										errors: fnFact.Errors,
										checked:   make(map[string]bool),
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

// collectErrorsIsInExpr finds errors.Is/As calls in an expression and marks errors as checked.
func collectErrorsIsInExpr(pass *analysis.Pass, expr ast.Expr, states map[*types.Var]*errorVarState) {
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if isErrorsIsCall(pass, call) {
			if len(call.Args) >= 2 {
				// Find which error variable is being checked
				for varObj, state := range states {
					if referencesVariable(pass, call.Args[0], varObj) {
						errorKey := extractErrorKey(pass, call.Args[1])
						if errorKey != "" {
							state.checked[errorKey] = true
						}
					}
				}
			}
		}

		if isErrorsAsCall(pass, call) {
			if len(call.Args) >= 2 {
				for varObj, state := range states {
					if referencesVariable(pass, call.Args[0], varObj) {
						errorKey := extractErrorKeyFromAsTarget(pass, call.Args[1])
						if errorKey != "" {
							state.checked[errorKey] = true
						}
					}
				}
			}
		}

		return true
	})
}

// reportUncheckedErrors reports any errors that haven't been checked.
func reportUncheckedErrors(pass *analysis.Pass, state *errorVarState) {
	for _, errInfo := range state.errors {
		key := errInfo.Key()
		if !state.checked[key] {
			pass.Reportf(state.callPos, "missing errors.Is check for %s", key)
		}
	}
}

// cloneStates creates a deep copy of the states map.
func cloneStates(states map[*types.Var]*errorVarState) map[*types.Var]*errorVarState {
	result := make(map[*types.Var]*errorVarState)
	for varObj, state := range states {
		newChecked := make(map[string]bool)
		for k, v := range state.checked {
			newChecked[k] = v
		}
		result[varObj] = &errorVarState{
			callPos: state.callPos,
			errors:  state.errors, // Slice is fine to share as we don't modify it
			checked: newChecked,
		}
	}
	return result
}

// mergeStates merges branch states back into the main states.
// A error is considered checked if it's checked in EITHER branch (conservative approach).
func mergeStates(states map[*types.Var]*errorVarState, ifStates, elseStates map[*types.Var]*errorVarState) {
	for varObj, state := range states {
		ifState := ifStates[varObj]
		elseState := elseStates[varObj]

		// A error is checked if checked in either branch
		for _, errInfo := range state.errors {
			key := errInfo.Key()
			checkedInIf := ifState != nil && ifState.checked[key]
			checkedInElse := elseState != nil && elseState.checked[key]
			if checkedInIf || checkedInElse {
				state.checked[key] = true
			}
		}
	}
}

// getCallErrors returns the FunctionErrorsFact and signature for a call expression.
// It handles both regular function calls and closure variable calls.
// It also resolves errors through ParameterFlowFact.
func getCallErrors(pass *analysis.Pass, call *ast.CallExpr) (*FunctionErrorsFact, *types.Signature) {
	// First, try to get it as a regular function
	calledFn := getCalledFunction(pass, call)
	if calledFn != nil {
		sig := calledFn.Type().(*types.Signature)

		// Start with FunctionErrorsFact
		result := &FunctionErrorsFact{}
		var fnFact FunctionErrorsFact
		if pass.ImportObjectFact(calledFn, &fnFact) {
			result.Merge(&fnFact)
		}

		// Also check for InterfaceMethodFact (for interface method calls)
		var ifaceFact InterfaceMethodFact
		if pass.ImportObjectFact(calledFn, &ifaceFact) {
			for _, err := range ifaceFact.Errors {
				result.AddError(err)
			}
		}

		// Also resolve errors through ParameterFlowFact
		var flowFact ParameterFlowFact
		if pass.ImportObjectFact(calledFn, &flowFact) {
			paramFlowErrors := resolveParameterFlowErrorsAST(pass, call, &flowFact)
			for _, err := range paramFlowErrors {
				result.AddError(err)
			}
		}

		// Also resolve errors through FunctionParamCallFlowFact (for higher-order functions)
		var callFlowFact FunctionParamCallFlowFact
		if pass.ImportObjectFact(calledFn, &callFlowFact) {
			callFlowErrors := resolveFunctionParamCallFlowErrors(pass, call, &callFlowFact)
			for _, err := range callFlowErrors {
				result.AddError(err)
			}
		}

		if len(result.Errors) > 0 {
			return result, sig
		}

		// If no facts found yet, check if this is an interface method and look for implementations
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if fact, _ := getInterfaceMethodErrors(pass, sel); fact != nil && len(fact.Errors) > 0 {
				return fact, sig
			}
		}

		return nil, nil
	}

	// Check for interface method call (selector expression on interface type)
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if fact, sig := getInterfaceMethodErrors(pass, sel); fact != nil {
			return fact, sig
		}
	}

	// Try to get it as a closure variable
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, nil
	}

	obj := pass.TypesInfo.Uses[ident]
	varObj, ok := obj.(*types.Var)
	if !ok {
		return nil, nil
	}

	// Check if the variable has a FunctionErrorsFact
	var fnFact FunctionErrorsFact
	if pass.ImportObjectFact(varObj, &fnFact) {
		// Get the signature from the variable's type
		sig, ok := varObj.Type().Underlying().(*types.Signature)
		if !ok {
			return nil, nil
		}
		return &fnFact, sig
	}

	return nil, nil
}

// getInterfaceMethodErrors returns errors from an interface method call.
// It checks if the receiver is an interface type and returns the InterfaceMethodFact.
func getInterfaceMethodErrors(pass *analysis.Pass, sel *ast.SelectorExpr) (*FunctionErrorsFact, *types.Signature) {
	// Check if the receiver is an interface type
	tv := pass.TypesInfo.Types[sel.X]
	if !tv.IsValue() {
		return nil, nil
	}

	ifaceType, ok := tv.Type.Underlying().(*types.Interface)
	if !ok {
		return nil, nil
	}

	// Get the method object
	methodObj := pass.TypesInfo.Uses[sel.Sel]
	method, ok := methodObj.(*types.Func)
	if !ok {
		return nil, nil
	}

	// Get the method signature
	sig, ok := method.Type().(*types.Signature)
	if !ok {
		return nil, nil
	}

	// Check for InterfaceMethodFact
	var ifaceFact InterfaceMethodFact
	if pass.ImportObjectFact(method, &ifaceFact) {
		return &FunctionErrorsFact{Errors: ifaceFact.Errors}, sig
	}

	// If no fact found, try to find implementations in the current package
	// This handles the case where the interface and implementations are in the same package
	impls := findInterfaceImplementations(pass)
	implementingTypes := impls.getImplementingTypes(ifaceType)

	result := &FunctionErrorsFact{}
	for _, concreteType := range implementingTypes {
		concreteMethod := findMethodImplementation(concreteType, method)
		if concreteMethod == nil {
			continue
		}

		var fnFact FunctionErrorsFact
		if pass.ImportObjectFact(concreteMethod, &fnFact) {
			result.Merge(&fnFact)
		}
	}

	if len(result.Errors) > 0 {
		return result, sig
	}

	return nil, nil
}

// flowInfo represents a parameter flow with index and wrapped flag.
// This interface abstracts the commonality between ParameterFlowInfo and FunctionParamCallFlowInfo.
type flowInfo interface {
	Index() int
	IsWrapped() bool
}

// paramFlowAdapter adapts ParameterFlowInfo to flowInfo interface.
type paramFlowAdapter struct{ f ParameterFlowInfo }

func (a paramFlowAdapter) Index() int       { return a.f.ParamIndex }
func (a paramFlowAdapter) IsWrapped() bool  { return a.f.Wrapped }

// funcParamCallFlowAdapter adapts FunctionParamCallFlowInfo to flowInfo interface.
type funcParamCallFlowAdapter struct{ f FunctionParamCallFlowInfo }

func (a funcParamCallFlowAdapter) Index() int       { return a.f.ParamIndex }
func (a funcParamCallFlowAdapter) IsWrapped() bool  { return a.f.Wrapped }

// resolveFlowErrors resolves errors from call arguments based on flow information.
func resolveFlowErrors(pass *analysis.Pass, call *ast.CallExpr, flows []flowInfo) []ErrorInfo {
	var errs []ErrorInfo

	for _, flow := range flows {
		if flow.Index() >= len(call.Args) {
			continue
		}

		arg := call.Args[flow.Index()]
		argErrors := extractErrorsFromExpr(pass, arg)

		for _, err := range argErrors {
			if flow.IsWrapped() {
				err.Wrapped = true
			}
			errs = append(errs, err)
		}
	}

	return errs
}

// resolveParameterFlowErrorsAST resolves errors from call arguments based on ParameterFlowFact.
func resolveParameterFlowErrorsAST(pass *analysis.Pass, call *ast.CallExpr, flowFact *ParameterFlowFact) []ErrorInfo {
	flows := make([]flowInfo, len(flowFact.Flows))
	for i, f := range flowFact.Flows {
		flows[i] = paramFlowAdapter{f}
	}
	return resolveFlowErrors(pass, call, flows)
}

// extractErrorsFromExpr extracts known errors from an expression.
func extractErrorsFromExpr(pass *analysis.Pass, expr ast.Expr) []ErrorInfo {
	var errs []ErrorInfo

	switch e := expr.(type) {
	case *ast.Ident:
		obj := pass.TypesInfo.Uses[e]
		if varObj, ok := obj.(*types.Var); ok {
			var errorFact ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				errs = append(errs, ErrorInfo{
					PkgPath: errorFact.PkgPath,
					Name:    errorFact.Name,
					Wrapped: false,
				})
			} else if varObj.Pkg() != nil && isErrorType(varObj.Type()) {
				// Local error variable
				errs = append(errs, ErrorInfo{
					PkgPath: varObj.Pkg().Path(),
					Name:    varObj.Name(),
					Wrapped: false,
				})
			}
			// Also check for FunctionErrorsFact (for closure variables)
			var fnFact FunctionErrorsFact
			if pass.ImportObjectFact(varObj, &fnFact) {
				errs = append(errs, fnFact.Errors...)
			}
		}

	case *ast.SelectorExpr:
		obj := pass.TypesInfo.Uses[e.Sel]
		if varObj, ok := obj.(*types.Var); ok {
			var errorFact ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				errs = append(errs, ErrorInfo{
					PkgPath: errorFact.PkgPath,
					Name:    errorFact.Name,
					Wrapped: false,
				})
			}
		}

	case *ast.CallExpr:
		// If the argument is a function call, recursively get its errors
		calledFn := getCalledFunction(pass, e)
		if calledFn != nil {
			var fnFact FunctionErrorsFact
			if pass.ImportObjectFact(calledFn, &fnFact) {
				errs = append(errs, fnFact.Errors...)
			}
			// Also check ParameterFlowFact for chained wrappers
			var flowFact ParameterFlowFact
			if pass.ImportObjectFact(calledFn, &flowFact) {
				paramErrs := resolveParameterFlowErrorsAST(pass, e, &flowFact)
				errs = append(errs, paramErrs...)
			}
		}

	case *ast.UnaryExpr:
		if e.Op.String() == "&" {
			if compLit, ok := e.X.(*ast.CompositeLit); ok {
				errs = append(errs, extractErrorsFromCompositeLit(pass, compLit)...)
			}
		}

	case *ast.CompositeLit:
		errs = append(errs, extractErrorsFromCompositeLit(pass, e)...)

	case *ast.FuncLit:
		// Analyze lambda body directly to extract errors
		errs = append(errs, analyzeFuncLitErrors(pass, e)...)
	}

	return errs
}

// extractErrorsFromCompositeLit extracts error type information from a composite literal.
func extractErrorsFromCompositeLit(pass *analysis.Pass, compLit *ast.CompositeLit) []ErrorInfo {
	var errs []ErrorInfo

	tv := pass.TypesInfo.Types[compLit]
	if !tv.IsValue() {
		return nil
	}

	namedType := extractNamedType(tv.Type)
	if namedType == nil {
		return nil
	}

	typeName := namedType.Obj()
	if typeName == nil {
		return nil
	}

	var errorFact ErrorFact
	if pass.ImportObjectFact(typeName, &errorFact) {
		errs = append(errs, ErrorInfo{
			PkgPath: errorFact.PkgPath,
			Name:    errorFact.Name,
			Wrapped: false,
		})
	} else if typeName.Pkg() != nil {
		// Local custom error type
		errs = append(errs, ErrorInfo{
			PkgPath: typeName.Pkg().Path(),
			Name:    typeName.Name(),
			Wrapped: false,
		})
	}

	return errs
}

// analyzeFuncLitErrors analyzes a function literal (lambda) body to extract errors.
func analyzeFuncLitErrors(pass *analysis.Pass, funcLit *ast.FuncLit) []ErrorInfo {
	tv := pass.TypesInfo.Types[funcLit]
	if !tv.IsValue() {
		return nil
	}

	sig, ok := tv.Type.(*types.Signature)
	if !ok {
		return nil
	}

	errorPositions := findErrorReturnPositions(sig)
	if len(errorPositions) == 0 {
		return nil
	}

	var errs []ErrorInfo
	ast.Inspect(funcLit.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		for _, pos := range errorPositions {
			if pos < len(ret.Results) {
				errs = append(errs, extractErrorsFromExpr(pass, ret.Results[pos])...)
			}
		}
		return true
	})

	return errs
}

// resolveFunctionParamCallFlowErrors resolves errors from function parameters that are called.
// This handles higher-order functions like RunInTx(fn func() error) error { return fn() }
func resolveFunctionParamCallFlowErrors(pass *analysis.Pass, call *ast.CallExpr, flowFact *FunctionParamCallFlowFact) []ErrorInfo {
	flows := make([]flowInfo, len(flowFact.CallFlows))
	for i, f := range flowFact.CallFlows {
		flows[i] = funcParamCallFlowAdapter{f}
	}
	return resolveFlowErrors(pass, call, flows)
}

// findErrorVarInAssignmentWithSig finds the error variable in an assignment statement using a signature.
func findErrorVarInAssignmentWithSig(pass *analysis.Pass, stmt *ast.AssignStmt, rhsIndex int, sig *types.Signature) *types.Var {
	results := sig.Results()

	// Handle multiple return values
	if len(stmt.Rhs) == 1 && results.Len() > 1 {
		// Single call with multiple returns: x, err := someFunc()
		for i := 0; i < results.Len(); i++ {
			if isErrorType(results.At(i).Type()) {
				if i < len(stmt.Lhs) {
					ident, ok := stmt.Lhs[i].(*ast.Ident)
					if !ok || ident.Name == "_" {
						continue
					}
					obj := pass.TypesInfo.Defs[ident]
					if obj == nil {
						obj = pass.TypesInfo.Uses[ident]
					}
					if varObj, ok := obj.(*types.Var); ok {
						return varObj
					}
				}
			}
		}
	} else {
		// Direct assignment: err := someFunc()
		if rhsIndex < len(stmt.Lhs) {
			ident, ok := stmt.Lhs[rhsIndex].(*ast.Ident)
			if !ok || ident.Name == "_" {
				return nil
			}
			obj := pass.TypesInfo.Defs[ident]
			if obj == nil {
				obj = pass.TypesInfo.Uses[ident]
			}
			if varObj, ok := obj.(*types.Var); ok {
				if isErrorType(varObj.Type()) {
					return varObj
				}
			}
		}
	}

	return nil
}

// isErrorsIsCall checks if the call is errors.Is().
func isErrorsIsCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	return isErrorsPkgCall(pass, call, "Is")
}

// isErrorsAsCall checks if the call is errors.As().
func isErrorsAsCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	return isErrorsPkgCall(pass, call, "As")
}

// isErrorsPkgCall checks if the call is errors.<funcName>().
func isErrorsPkgCall(pass *analysis.Pass, call *ast.CallExpr, funcName string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	if sel.Sel.Name != funcName {
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

	return pkgName.Imported().Path() == "errors"
}

// extractErrorKeyFromAsTarget extracts the error key from errors.As target.
// errors.As(err, &target) where target is *SomeErrorType
func extractErrorKeyFromAsTarget(pass *analysis.Pass, expr ast.Expr) string {
	// errors.As takes a pointer to the target, so we need to get the underlying type
	tv := pass.TypesInfo.Types[expr]
	if !tv.IsValue() {
		return ""
	}

	// The second argument should be **SomeErrorType or *interface type
	ptrType, ok := tv.Type.(*types.Pointer)
	if !ok {
		return ""
	}

	// Get the element type (should be *SomeErrorType or interface)
	elemType := ptrType.Elem()

	// If it's a pointer to a named type
	if innerPtr, ok := elemType.(*types.Pointer); ok {
		if named, ok := innerPtr.Elem().(*types.Named); ok {
			typeName := named.Obj()
			var errorFact ErrorFact
			if pass.ImportObjectFact(typeName, &errorFact) {
				return errorFact.PkgPath + "." + errorFact.Name
			}
			if typeName.Pkg() != nil {
				return typeName.Pkg().Path() + "." + typeName.Name()
			}
		}
	}

	// If it's a named type directly (non-pointer error type)
	if named, ok := elemType.(*types.Named); ok {
		typeName := named.Obj()
		var errorFact ErrorFact
		if pass.ImportObjectFact(typeName, &errorFact) {
			return errorFact.PkgPath + "." + errorFact.Name
		}
		if typeName.Pkg() != nil {
			return typeName.Pkg().Path() + "." + typeName.Name()
		}
	}

	return ""
}

// referencesVariable checks if an expression references the given variable.
func referencesVariable(pass *analysis.Pass, expr ast.Expr, targetVar *types.Var) bool {
	var found bool
	ast.Inspect(expr, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pass.TypesInfo.Uses[ident]
		if obj == targetVar {
			found = true
			return false
		}
		return true
	})
	return found
}

// extractErrorKey extracts the error key from an errors.Is second argument.
func extractErrorKey(pass *analysis.Pass, expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		obj := pass.TypesInfo.Uses[e]
		if varObj, ok := obj.(*types.Var); ok {
			var errorFact ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				return errorFact.PkgPath + "." + errorFact.Name
			}
			// For local errors in same package
			if varObj.Pkg() != nil {
				return varObj.Pkg().Path() + "." + varObj.Name()
			}
		}

	case *ast.SelectorExpr:
		obj := pass.TypesInfo.Uses[e.Sel]
		if varObj, ok := obj.(*types.Var); ok {
			var errorFact ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				return errorFact.PkgPath + "." + errorFact.Name
			}
			if varObj.Pkg() != nil {
				return varObj.Pkg().Path() + "." + varObj.Name()
			}
		}
	}

	return ""
}

