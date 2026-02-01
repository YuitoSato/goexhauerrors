package checker

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/YuitoSato/goexhauerrors/goexhauerrors/facts"
	"github.com/YuitoSato/goexhauerrors/goexhauerrors/internal"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// CallSiteAnalyzer holds context for call site analysis to avoid recomputing expensive data.
type CallSiteAnalyzer struct {
	Pass               *analysis.Pass
	InterfaceImpls     *internal.InterfaceImplementations
	hadGlobalStoreMiss bool                          // set to true if any interface method lookup missed the global store
	reported           map[token.Pos]map[string]bool // tracks (callPos, errorKey) already reported to prevent duplicates
}

// CheckCallSites checks all call sites to ensure errors are properly checked.
func CheckCallSites(pass *analysis.Pass, interfaceImpls *internal.InterfaceImplementations) {
	checkCallSitesInternal(pass, interfaceImpls, true)
}

func checkCallSitesInternal(pass *analysis.Pass, interfaceImpls *internal.InterfaceImplementations, registerDeferred bool) {
	csa := &CallSiteAnalyzer{
		Pass:           pass,
		InterfaceImpls: interfaceImpls,
		reported:       make(map[token.Pos]map[string]bool),
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Collect functions that had global store misses for deferred re-analysis
	type funcToCheck struct {
		body         *ast.BlockStmt
		returnsError bool
	}
	var missedFuncs []funcToCheck

	nodeFilter := []ast.Node{
		(*ast.FuncDecl)(nil),
		(*ast.FuncLit)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Body == nil {
				return
			}
			returnsError := funcReturnsError(pass, node)
			csa.hadGlobalStoreMiss = false
			csa.checkFunctionBody(node.Body, returnsError)
			if csa.hadGlobalStoreMiss && registerDeferred {
				missedFuncs = append(missedFuncs, funcToCheck{node.Body, returnsError})
			}

		case *ast.FuncLit:
			if node.Body == nil {
				return
			}
			tv := pass.TypesInfo.Types[node]
			if !tv.IsValue() {
				return
			}
			sig, ok := tv.Type.(*types.Signature)
			if !ok {
				return
			}
			returnsError := false
			results := sig.Results()
			for i := 0; i < results.Len(); i++ {
				if internal.IsErrorType(results.At(i).Type()) {
					returnsError = true
					break
				}
			}
			csa.hadGlobalStoreMiss = false
			csa.checkFunctionBody(node.Body, returnsError)
			if csa.hadGlobalStoreMiss && registerDeferred {
				missedFuncs = append(missedFuncs, funcToCheck{node.Body, returnsError})
			}
		}
	})

	// Register deferred re-analysis for functions that had global store misses
	if len(missedFuncs) > 0 && registerDeferred {
		capturedPass := pass
		capturedImpls := interfaceImpls
		capturedFuncs := missedFuncs
		capturedReported := csa.reported
		facts.AddDeferredFunctionCheck(&facts.DeferredFunctionCheck{
			ReAnalyze: func() bool {
				reAnalyzer := &CallSiteAnalyzer{
					Pass:           capturedPass,
					InterfaceImpls: capturedImpls,
					reported:       capturedReported,
				}
				stillHasMiss := false
				for _, f := range capturedFuncs {
					reAnalyzer.hadGlobalStoreMiss = false
					reAnalyzer.checkFunctionBody(f.body, f.returnsError)
					if reAnalyzer.hadGlobalStoreMiss {
						stillHasMiss = true
					}
				}
				return stillHasMiss
			},
		})
	}
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
		if internal.IsErrorType(results.At(i).Type()) {
			return true
		}
	}
	return false
}

// errorVarState tracks the active errors for an error variable.
type errorVarState struct {
	callPos          token.Pos
	errors           []facts.ErrorInfo
	checked          map[string]bool
	propagatableKeys map[string]bool // if non-nil, only these error keys can be propagated via return
}

// checkFunctionBody checks all call sites within a function body using flow-sensitive analysis.
func (csa *CallSiteAnalyzer) checkFunctionBody(body *ast.BlockStmt, canPropagate bool) {
	// Track active error states for each variable
	states := make(map[*types.Var]*errorVarState)

	// Walk statements in order for flow-sensitive analysis
	csa.walkStatementsWithScope(body.List, states, canPropagate)

	// Report any remaining unchecked errors at end of function
	for _, state := range states {
		reportUncheckedErrors(csa.Pass, state, csa.reported)
	}
}

// walkStatementsWithScope walks statements and tracks error variable states.
func (csa *CallSiteAnalyzer) walkStatementsWithScope(stmts []ast.Stmt, states map[*types.Var]*errorVarState, canPropagate bool) {
	for _, stmt := range stmts {
		csa.walkStatementWithScope(stmt, states, canPropagate)
	}
}

// walkStatementWithScope processes a single statement for error tracking.
func (csa *CallSiteAnalyzer) walkStatementWithScope(stmt ast.Stmt, states map[*types.Var]*errorVarState, canPropagate bool) {
	pass := csa.Pass
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

			// Mark tracked error variables as transferred when passed to functions with ParameterFlowFact
			csa.markTransferredErrorArgs(call, states)

			fnFact, sig := csa.getCallErrors(call)
			if fnFact == nil || len(fnFact.Errors) == 0 {
				continue
			}

			errorVar := findErrorVarInAssignmentWithSig(pass, s, i, sig)
			if errorVar == nil {
				continue
			}

			// If this variable already has active errors, report unchecked ones
			if existingState, ok := states[errorVar]; ok {
				reportUncheckedErrors(pass, existingState, csa.reported)
			}

			// Set new state for this variable
			states[errorVar] = &errorVarState{
				callPos: call.Pos(),
				errors:  fnFact.Errors,
				checked: make(map[string]bool),
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
			csa.walkStatementWithScope(s.Init, states, canPropagate)
		}

		// Clone states for branches
		ifStates := cloneStates(states)
		elseStates := cloneStates(states)

		// Process if body
		csa.walkStatementsWithScope(s.Body.List, ifStates, canPropagate)

		// Process else body if present
		if s.Else != nil {
			switch elseStmt := s.Else.(type) {
			case *ast.BlockStmt:
				csa.walkStatementsWithScope(elseStmt.List, elseStates, canPropagate)
			case *ast.IfStmt:
				csa.walkStatementWithScope(elseStmt, elseStates, canPropagate)
			}
		}

		// Merge states back
		mergeStates(states, ifStates, elseStates)

	case *ast.SwitchStmt:
		if s.Init != nil {
			csa.walkStatementWithScope(s.Init, states, canPropagate)
		}
		if s.Tag != nil {
			collectErrorsIsInExpr(pass, s.Tag, states)
		}

		// Find tracked error variable used as switch tag (for `switch err { case ErrX: }`)
		var switchTagVar *types.Var
		if s.Tag != nil {
			if ident, ok := s.Tag.(*ast.Ident); ok {
				obj := pass.TypesInfo.Uses[ident]
				if v, ok := obj.(*types.Var); ok {
					if _, tracked := states[v]; tracked {
						switchTagVar = v
					}
				}
			}
		}

		// Process switch body
		if s.Body != nil {
			for _, clause := range s.Body.List {
				if cc, ok := clause.(*ast.CaseClause); ok {
					// Clone states FIRST so case condition checks are scoped to this case only
					caseStates := cloneStates(states)

					// Check case expressions for errors.Is (scoped to caseStates)
					for _, expr := range cc.List {
						collectErrorsIsInExpr(pass, expr, caseStates)
					}
					// Check case values as direct comparisons against switch tag
					if switchTagVar != nil {
						if state, ok := caseStates[switchTagVar]; ok {
							for _, expr := range cc.List {
								errorKey := internal.ExtractErrorKey(pass, expr)
								if errorKey != "" {
									state.checked[errorKey] = true
								}
							}
						}
					}

					applyPropagationNarrowing(caseStates, states)

					// Walk case body
					csa.walkStatementsWithScope(cc.Body, caseStates, canPropagate)
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

	case *ast.TypeSwitchStmt:
		if s.Init != nil {
			csa.walkStatementWithScope(s.Init, states, canPropagate)
		}

		// Find the error variable being type-switched
		var switchVar *types.Var
		if s.Assign != nil {
			switch assign := s.Assign.(type) {
			case *ast.ExprStmt:
				// switch err.(type) { ... }
				if ta, ok := assign.X.(*ast.TypeAssertExpr); ok {
					if ident, ok := ta.X.(*ast.Ident); ok {
						obj := pass.TypesInfo.Uses[ident]
						if v, ok := obj.(*types.Var); ok {
							switchVar = v
						}
					}
				}
			case *ast.AssignStmt:
				// switch v := err.(type) { ... }
				if len(assign.Rhs) == 1 {
					if ta, ok := assign.Rhs[0].(*ast.TypeAssertExpr); ok {
						if ident, ok := ta.X.(*ast.Ident); ok {
							obj := pass.TypesInfo.Uses[ident]
							if v, ok := obj.(*types.Var); ok {
								switchVar = v
							}
						}
					}
				}
			}
		}

		if s.Body != nil {
			for _, clause := range s.Body.List {
				if cc, ok := clause.(*ast.CaseClause); ok {
					// Clone states FIRST so type checks are scoped to this case only
					caseStates := cloneStates(states)

					// Check if any case type matches a tracked error type
					if switchVar != nil {
						if state, ok := caseStates[switchVar]; ok {
							for _, caseExpr := range cc.List {
								typeName := internal.ExtractTypeNameFromExpr(pass, caseExpr)
								if typeName != "" {
									state.checked[typeName] = true
								}
							}
						}
					}

					applyPropagationNarrowing(caseStates, states)

					// Walk case body
					csa.walkStatementsWithScope(cc.Body, caseStates, canPropagate)
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
				if csa.isVariablePropagatedInReturn(result, varObj) {
					if canPropagate {
						// Mark errors as "checked" since we're propagating.
						// If propagatableKeys is set (inside a switch case with errors.Is narrowing),
						// only propagate the narrowed errors, not all errors.
						for _, errInfo := range state.errors {
							key := errInfo.Key()
							if state.propagatableKeys == nil || state.propagatableKeys[key] {
								state.checked[key] = true
							}
						}
					}
				} else {
					// Not fully propagated - check for partial checks via ParameterCheckedErrorsFact
					csa.markCheckedErrorsFromCall(result, varObj, state)
				}
			}
		}

	case *ast.BlockStmt:
		csa.walkStatementsWithScope(s.List, states, canPropagate)

	case *ast.ForStmt:
		if s.Init != nil {
			csa.walkStatementWithScope(s.Init, states, canPropagate)
		}
		if s.Cond != nil {
			collectErrorsIsInExpr(pass, s.Cond, states)
		}
		if s.Post != nil {
			csa.walkStatementWithScope(s.Post, states, canPropagate)
		}
		if s.Body != nil {
			csa.walkStatementsWithScope(s.Body.List, states, canPropagate)
		}

	case *ast.RangeStmt:
		if s.Body != nil {
			csa.walkStatementsWithScope(s.Body.List, states, canPropagate)
		}

	case *ast.DeferStmt:
		// Check the deferred call expression for errors.Is/As
		collectErrorsIsInExpr(pass, s.Call, states)

	case *ast.SelectStmt:
		if s.Body != nil {
			for _, clause := range s.Body.List {
				if cc, ok := clause.(*ast.CommClause); ok {
					caseStates := cloneStates(states)
					csa.walkStatementsWithScope(cc.Body, caseStates, canPropagate)
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

	case *ast.DeclStmt:
		if genDecl, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range genDecl.Specs {
				if valueSpec, ok := spec.(*ast.ValueSpec); ok {
					for i, value := range valueSpec.Values {
						call, ok := value.(*ast.CallExpr)
						if !ok {
							continue
						}

						fnFact, _ := csa.getCallErrors(call)
						if fnFact == nil || len(fnFact.Errors) == 0 {
							continue
						}

						if i < len(valueSpec.Names) {
							obj := pass.TypesInfo.Defs[valueSpec.Names[i]]
							if varObj, ok := obj.(*types.Var); ok {
								if internal.IsErrorType(varObj.Type()) {
									states[varObj] = &errorVarState{
										callPos: call.Pos(),
										errors:  fnFact.Errors,
										checked: make(map[string]bool),
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

// collectErrorsIsInExpr finds errors.Is/As calls and direct comparisons in an expression and marks errors as checked.
func collectErrorsIsInExpr(pass *analysis.Pass, expr ast.Expr, states map[*types.Var]*errorVarState) {
	ast.Inspect(expr, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if internal.IsErrorsIsCall(pass, node) {
				if len(node.Args) >= 2 {
					// Find which error variable is being checked
					for varObj, state := range states {
						if internal.ReferencesVariable(pass, node.Args[0], varObj) {
							errorKey := internal.ExtractErrorKey(pass, node.Args[1])
							if errorKey != "" {
								state.checked[errorKey] = true
							}
						}
					}
				}
			}

			if internal.IsErrorsAsCall(pass, node) {
				if len(node.Args) >= 2 {
					for varObj, state := range states {
						if internal.ReferencesVariable(pass, node.Args[0], varObj) {
							errorKey := internal.ExtractErrorKeyFromAsTarget(pass, node.Args[1])
							if errorKey != "" {
								state.checked[errorKey] = true
							}
						}
					}
				}
			}

		case *ast.BinaryExpr:
			if node.Op == token.EQL || node.Op == token.NEQ {
				// Try both directions: err == ErrX or ErrX == err
				tryMarkDirectComparison(pass, node.X, node.Y, states)
				tryMarkDirectComparison(pass, node.Y, node.X, states)
			}
		}

		return true
	})
}

// tryMarkDirectComparison checks if lhs is a tracked error variable and rhs is a known error,
// and marks the error as checked if so.
func tryMarkDirectComparison(pass *analysis.Pass, lhs, rhs ast.Expr, states map[*types.Var]*errorVarState) {
	for varObj, state := range states {
		if internal.ReferencesVariable(pass, lhs, varObj) {
			errorKey := internal.ExtractErrorKey(pass, rhs)
			if errorKey != "" {
				state.checked[errorKey] = true
			}
		}
	}
}

// isVariablePropagatedInReturn checks if a variable's errors are propagated
// through a return expression. It distinguishes between:
// - Direct return (return err) -> propagation
// - Function call with ParameterFlowFact (return WrapError(err)) -> propagation
// - fmt.Errorf with %w (return fmt.Errorf("...: %w", err)) -> propagation
// - Function call without ParameterFlowFact (return ConsumeError(err)) -> NOT propagation
func (csa *CallSiteAnalyzer) isVariablePropagatedInReturn(result ast.Expr, targetVar *types.Var) bool {
	pass := csa.Pass
	switch expr := result.(type) {
	case *ast.Ident:
		// Direct return: return err
		obj := pass.TypesInfo.Uses[expr]
		return obj == targetVar

	case *ast.CallExpr:
		// Check if the variable is used as an argument
		argIndex := internal.FindArgIndexForVar(pass, expr, targetVar)
		if argIndex < 0 {
			// Variable not found as a direct argument - fall back to referencesVariable
			return internal.ReferencesVariable(pass, result, targetVar)
		}

		// Special case: fmt.Errorf with %w
		if internal.IsFmtErrorfCall(pass, expr) {
			return internal.IsFmtErrorfWrappingVariable(pass, expr, targetVar)
		}

		// Check if the called function has ParameterFlowFact for this argument
		calledFn := internal.GetCalledFunction(pass, expr)
		if calledFn != nil {
			var flowFact facts.ParameterFlowFact
			if pass.ImportObjectFact(calledFn, &flowFact) {
				return flowFact.HasFlowForParam(argIndex)
			}
			// Fallback for cross-package interface methods
			if sel, ok := expr.Fun.(*ast.SelectorExpr); ok {
				if ifaceFlowFact := csa.getInterfaceMethodParameterFlow(sel); ifaceFlowFact != nil {
					return ifaceFlowFact.HasFlowForParam(argIndex)
				}
			}
			// Function was analyzed but has no ParameterFlowFact -> NOT propagation
			return false
		}

		// Unknown function (e.g., closure variable) - conservative: treat as propagation
		return true

	default:
		// For other expressions, fall back to referencesVariable
		return internal.ReferencesVariable(pass, result, targetVar)
	}
}

// markTransferredErrorArgs marks tracked error variables as checked when they are
// passed to a function that has ParameterFlowFact or ParameterCheckedErrorsFact.
// - ParameterFlowFact: marks ALL errors as transferred (full propagation)
// - ParameterCheckedErrorsFact: marks only the checked errors (partial check)
func (csa *CallSiteAnalyzer) markTransferredErrorArgs(call *ast.CallExpr, states map[*types.Var]*errorVarState) {
	pass := csa.Pass
	calledFn := internal.GetCalledFunction(pass, call)
	if calledFn == nil {
		return
	}

	var flowFact facts.ParameterFlowFact
	hasFlowFact := pass.ImportObjectFact(calledFn, &flowFact)

	var checkedFact facts.ParameterCheckedErrorsFact
	hasCheckedFact := pass.ImportObjectFact(calledFn, &checkedFact)

	// Fallback for cross-package interface methods: dynamically compute from implementations
	if !hasFlowFact || !hasCheckedFact {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if !hasFlowFact {
				if ifaceFlowFact := csa.getInterfaceMethodParameterFlow(sel); ifaceFlowFact != nil {
					flowFact = *ifaceFlowFact
					hasFlowFact = true
				}
			}
			if !hasCheckedFact {
				if ifaceCheckedFact := csa.getInterfaceMethodCheckedErrors(sel); ifaceCheckedFact != nil {
					checkedFact = *ifaceCheckedFact
					hasCheckedFact = true
				}
			}
		}
	}

	if !hasFlowFact && !hasCheckedFact {
		return
	}

	for i, arg := range call.Args {
		ident, ok := arg.(*ast.Ident)
		if !ok {
			continue
		}
		obj := pass.TypesInfo.Uses[ident]
		varObj, ok := obj.(*types.Var)
		if !ok {
			continue
		}
		state, ok := states[varObj]
		if !ok {
			continue
		}
		// If this argument position has a parameter flow, mark ALL errors as transferred
		if hasFlowFact && flowFact.HasFlowForParam(i) {
			for _, errInfo := range state.errors {
				state.checked[errInfo.Key()] = true
			}
		} else if hasCheckedFact {
			// Mark only the specific errors that are checked inside the function
			markPartialCheckedErrors(state, &checkedFact, i)
		}
	}
}

// markCheckedErrorsFromCall checks if a return expression is a function call
// with ParameterCheckedErrorsFact and marks the checked errors on the variable's state.
func (csa *CallSiteAnalyzer) markCheckedErrorsFromCall(result ast.Expr, targetVar *types.Var, state *errorVarState) {
	pass := csa.Pass
	call, ok := result.(*ast.CallExpr)
	if !ok {
		return
	}

	argIndex := internal.FindArgIndexForVar(pass, call, targetVar)
	if argIndex < 0 {
		return
	}

	calledFn := internal.GetCalledFunction(pass, call)
	if calledFn == nil {
		return
	}

	var checkedFact facts.ParameterCheckedErrorsFact
	if pass.ImportObjectFact(calledFn, &checkedFact) {
		markPartialCheckedErrors(state, &checkedFact, argIndex)
	} else if sel, ok := result.(*ast.CallExpr).Fun.(*ast.SelectorExpr); ok {
		// Fallback for cross-package interface methods
		if ifaceCheckedFact := csa.getInterfaceMethodCheckedErrors(sel); ifaceCheckedFact != nil {
			markPartialCheckedErrors(state, ifaceCheckedFact, argIndex)
		}
	}
}

// markPartialCheckedErrors marks specific errors as checked based on ParameterCheckedErrorsFact.
func markPartialCheckedErrors(state *errorVarState, checkedFact *facts.ParameterCheckedErrorsFact, argIndex int) {
	checkedErrors := checkedFact.GetCheckedErrors(argIndex)
	for _, checkedErr := range checkedErrors {
		checkedKey := checkedErr.Key()
		for _, errInfo := range state.errors {
			if errInfo.Key() == checkedKey {
				state.checked[checkedKey] = true
			}
		}
	}
}

// reportUncheckedErrors reports any errors that haven't been checked.
// The reported map tracks (callPos, errorKey) pairs already reported to prevent
// duplicate diagnostics when deferred re-analysis re-walks the same function body.
func reportUncheckedErrors(pass *analysis.Pass, state *errorVarState, reported map[token.Pos]map[string]bool) {
	for _, errInfo := range state.errors {
		// Skip ignored packages
		if internal.ShouldIgnorePackage(errInfo.PkgPath) {
			continue
		}
		// Skip unexported errors from other packages.
		// These cannot be checked with errors.Is/errors.As from outside the package.
		if errInfo.PkgPath != pass.Pkg.Path() && !token.IsExported(errInfo.Name) {
			continue
		}
		key := errInfo.Key()
		if !state.checked[key] {
			// Skip if already reported (prevents duplicates across first-pass and deferred re-analysis)
			if reported != nil {
				if reported[state.callPos][key] {
					continue
				}
				if reported[state.callPos] == nil {
					reported[state.callPos] = make(map[string]bool)
				}
				reported[state.callPos][key] = true
			}
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
		var newPropKeys map[string]bool
		if state.propagatableKeys != nil {
			newPropKeys = make(map[string]bool)
			for k, v := range state.propagatableKeys {
				newPropKeys[k] = v
			}
		}
		result[varObj] = &errorVarState{
			callPos:          state.callPos,
			errors:           state.errors, // Slice is fine to share as we don't modify it
			checked:          newChecked,
			propagatableKeys: newPropKeys,
		}
	}
	return result
}

// applyPropagationNarrowing sets propagatableKeys on caseStates based on
// which errors were newly checked by the case condition (not present in parent states).
// If the case narrows to specific errors (via errors.Is or direct comparison),
// only those errors will be propagatable via return in the case body.
// If no narrowing is detected (catch-all), propagatableKeys stays nil, allowing all.
func applyPropagationNarrowing(caseStates, parentStates map[*types.Var]*errorVarState) {
	for varObj, caseState := range caseStates {
		if parentState, ok := parentStates[varObj]; ok {
			narrowed := make(map[string]bool)
			for key := range caseState.checked {
				if !parentState.checked[key] {
					narrowed[key] = true
				}
			}
			if len(narrowed) > 0 {
				caseState.propagatableKeys = narrowed
			}
		}
	}
}

// mergeStates merges branch states back into the main states.
// An error is considered checked if it's checked in EITHER branch (OR merge).
// This is the pragmatic choice for Go's control flow model where early returns
// (if err != nil { return err }) mean only one branch continues after the if.
// AND merge would be theoretically more precise but breaks idiomatic Go patterns.
func mergeStates(states map[*types.Var]*errorVarState, ifStates, elseStates map[*types.Var]*errorVarState) {
	for varObj, state := range states {
		ifState := ifStates[varObj]
		elseState := elseStates[varObj]

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
func (csa *CallSiteAnalyzer) getCallErrors(call *ast.CallExpr) (*facts.FunctionErrorsFact, *types.Signature) {
	pass := csa.Pass
	// First, try to get it as a regular function
	calledFn := internal.GetCalledFunction(pass, call)
	if calledFn != nil {
		sig := calledFn.Type().(*types.Signature)

		// Start with FunctionErrorsFact
		result := &facts.FunctionErrorsFact{}
		var fnFact facts.FunctionErrorsFact
		if pass.ImportObjectFact(calledFn, &fnFact) {
			result.Merge(&fnFact)
		}

		// Also check for InterfaceMethodFact (for interface method calls)
		var ifaceFact facts.InterfaceMethodFact
		if pass.ImportObjectFact(calledFn, &ifaceFact) {
			for _, err := range ifaceFact.Errors {
				result.AddError(err)
			}
		}

		// Also resolve errors through ParameterFlowFact
		var flowFact facts.ParameterFlowFact
		if pass.ImportObjectFact(calledFn, &flowFact) {
			paramFlowErrors := resolveParameterFlowErrorsAST(pass, call, &flowFact)
			for _, err := range paramFlowErrors {
				result.AddError(err)
			}
		} else if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			// Fallback for cross-package interface methods: dynamically compute
			// intersection of ParameterFlowFact from implementations
			if ifaceFlowFact := csa.getInterfaceMethodParameterFlow(sel); ifaceFlowFact != nil {
				paramFlowErrors := resolveParameterFlowErrorsAST(pass, call, ifaceFlowFact)
				for _, err := range paramFlowErrors {
					result.AddError(err)
				}
			}
		}

		// Also resolve errors through FunctionParamCallFlowFact (for higher-order functions)
		var callFlowFact facts.FunctionParamCallFlowFact
		if pass.ImportObjectFact(calledFn, &callFlowFact) {
			callFlowErrors := resolveFunctionParamCallFlowErrors(pass, call, &callFlowFact)
			for _, err := range callFlowErrors {
				result.AddError(err)
			}
		} else if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			// Fallback for cross-package interface methods: dynamically compute
			// intersection of FunctionParamCallFlowFact from implementations
			if ifaceCallFlowFact := csa.getInterfaceMethodCallFlow(sel); ifaceCallFlowFact != nil {
				callFlowErrors := resolveFunctionParamCallFlowErrors(pass, call, ifaceCallFlowFact)
				for _, err := range callFlowErrors {
					result.AddError(err)
				}
			}
		}

		// Merge InterfaceMethodFact from global store (handles DI pattern where
		// both call flow and direct errors come from the global store)
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ifaceFact, _ := csa.getInterfaceMethodErrors(sel); ifaceFact != nil && len(ifaceFact.Errors) > 0 {
				result.Merge(ifaceFact)
			}
		}

		if len(result.Errors) > 0 {
			return result, sig
		}

		return nil, nil
	}

	// Check for interface method call (selector expression on interface type)
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if fact, sig := csa.getInterfaceMethodErrors(sel); fact != nil {
			// Also resolve FunctionParamCallFlowFact for this interface method
			if callFlowFact := csa.getInterfaceMethodCallFlow(sel); callFlowFact != nil && len(callFlowFact.CallFlows) > 0 {
				callFlowErrors := resolveFunctionParamCallFlowErrors(pass, call, callFlowFact)
				for _, err := range callFlowErrors {
					fact.AddError(err)
				}
			}
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
	var fnFact facts.FunctionErrorsFact
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
func (csa *CallSiteAnalyzer) getInterfaceMethodErrors(sel *ast.SelectorExpr) (*facts.FunctionErrorsFact, *types.Signature) {
	ifaceType, method := csa.resolveInterfaceMethod(sel)
	if ifaceType == nil {
		return nil, nil
	}

	pass := csa.Pass

	sig, ok := method.Type().(*types.Signature)
	if !ok {
		return nil, nil
	}

	// Check for InterfaceMethodFact
	var ifaceFact facts.InterfaceMethodFact
	if pass.ImportObjectFact(method, &ifaceFact) {
		return &facts.FunctionErrorsFact{Errors: ifaceFact.Errors}, sig
	}

	// Fallback: check the global store for cross-package DI pattern.
	// This handles the case where the implementation package is not in the caller's import graph.
	if ifaceTypeName := findInterfaceTypeName(pass, ifaceType); ifaceTypeName != nil {
		key := facts.InterfaceMethodKey(ifaceTypeName.Pkg().Path(), ifaceTypeName.Name(), method.Name())
		if globalFact, ok := facts.LoadInterfaceMethodFact(key); ok {
			return &facts.FunctionErrorsFact{Errors: globalFact.Errors}, sig
		}
		// Mark that we had a global store miss so the caller can register for deferred re-analysis
		csa.hadGlobalStoreMiss = true
	}

	// If no fact found, try to find implementations in the current package
	// This handles the case where the interface and implementations are in the same package
	implementingTypes := csa.InterfaceImpls.GetImplementingTypes(ifaceType)

	result := &facts.FunctionErrorsFact{}
	for _, concreteType := range implementingTypes {
		concreteMethod := internal.FindMethodImplementation(concreteType, method)
		if concreteMethod == nil {
			continue
		}

		var fnFact facts.FunctionErrorsFact
		if pass.ImportObjectFact(concreteMethod, &fnFact) {
			result.Merge(&fnFact)
		}
	}

	if len(result.Errors) > 0 {
		return result, sig
	}

	return nil, nil
}

// findInterfaceTypeName finds the *types.TypeName for a given interface type
// by searching the package scope and imported package scopes.
func findInterfaceTypeName(pass *analysis.Pass, ifaceType *types.Interface) *types.TypeName {
	// Search current package scope
	if tn := findInterfaceTypeNameInScope(pass.Pkg.Scope(), ifaceType); tn != nil {
		return tn
	}
	// Search imported package scopes
	for _, imp := range pass.Pkg.Imports() {
		if tn := findInterfaceTypeNameInScope(imp.Scope(), ifaceType); tn != nil {
			return tn
		}
	}
	return nil
}

func findInterfaceTypeNameInScope(scope *types.Scope, ifaceType *types.Interface) *types.TypeName {
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		if iface, ok := tn.Type().Underlying().(*types.Interface); ok {
			if types.Identical(iface, ifaceType) {
				return tn
			}
		}
	}
	return nil
}

// resolveInterfaceMethod resolves the interface type and method from a selector expression.
// Returns nil for both if the receiver is not an interface type.
func (csa *CallSiteAnalyzer) resolveInterfaceMethod(sel *ast.SelectorExpr) (*types.Interface, *types.Func) {
	tv := csa.Pass.TypesInfo.Types[sel.X]
	if !tv.IsValue() {
		return nil, nil
	}

	ifaceType, ok := tv.Type.Underlying().(*types.Interface)
	if !ok {
		return nil, nil
	}

	methodObj := csa.Pass.TypesInfo.Uses[sel.Sel]
	method, ok := methodObj.(*types.Func)
	if !ok {
		return nil, nil
	}

	return ifaceType, method
}

// getInterfaceMethodParameterFlow returns the intersection of ParameterFlowFact
// from all implementations of an interface method.
// Returns nil if the receiver is not an interface type or no implementations have flow facts.
func (csa *CallSiteAnalyzer) getInterfaceMethodParameterFlow(sel *ast.SelectorExpr) *facts.ParameterFlowFact {
	ifaceType, method := csa.resolveInterfaceMethod(sel)
	if ifaceType == nil {
		return nil
	}

	// Check for ParameterFlowFact directly on the interface method
	var flowFact facts.ParameterFlowFact
	if csa.Pass.ImportObjectFact(method, &flowFact) {
		return &flowFact
	}

	// Dynamically compute intersection from implementations
	implementingTypes := csa.InterfaceImpls.GetImplementingTypes(ifaceType)
	if len(implementingTypes) == 0 {
		return nil
	}

	var allFlowFacts []*facts.ParameterFlowFact
	for _, concreteType := range implementingTypes {
		concreteMethod := internal.FindMethodImplementation(concreteType, method)
		if concreteMethod == nil {
			continue
		}
		var pf facts.ParameterFlowFact
		if csa.Pass.ImportObjectFact(concreteMethod, &pf) {
			allFlowFacts = append(allFlowFacts, &pf)
		} else {
			allFlowFacts = append(allFlowFacts, nil)
		}
	}

	return facts.IntersectParameterFlowFacts(allFlowFacts)
}

// getInterfaceMethodCheckedErrors returns the intersection of ParameterCheckedErrorsFact
// from all implementations of an interface method.
// Returns nil if the receiver is not an interface type or no implementations have checked facts.
func (csa *CallSiteAnalyzer) getInterfaceMethodCheckedErrors(sel *ast.SelectorExpr) *facts.ParameterCheckedErrorsFact {
	ifaceType, method := csa.resolveInterfaceMethod(sel)
	if ifaceType == nil {
		return nil
	}

	// Check for ParameterCheckedErrorsFact directly on the interface method
	var checkedFact facts.ParameterCheckedErrorsFact
	if csa.Pass.ImportObjectFact(method, &checkedFact) {
		return &checkedFact
	}

	// Dynamically compute intersection from implementations
	implementingTypes := csa.InterfaceImpls.GetImplementingTypes(ifaceType)
	if len(implementingTypes) == 0 {
		return nil
	}

	var allCheckedFacts []*facts.ParameterCheckedErrorsFact
	for _, concreteType := range implementingTypes {
		concreteMethod := internal.FindMethodImplementation(concreteType, method)
		if concreteMethod == nil {
			continue
		}
		var cf facts.ParameterCheckedErrorsFact
		if csa.Pass.ImportObjectFact(concreteMethod, &cf) {
			allCheckedFacts = append(allCheckedFacts, &cf)
		} else {
			allCheckedFacts = append(allCheckedFacts, nil)
		}
	}

	return facts.IntersectParameterCheckedErrorsFacts(allCheckedFacts)
}

// getInterfaceMethodCallFlow returns the intersection of FunctionParamCallFlowFact
// from all implementations of an interface method.
// Returns nil if the receiver is not an interface type or no implementations have call flow facts.
func (csa *CallSiteAnalyzer) getInterfaceMethodCallFlow(sel *ast.SelectorExpr) *facts.FunctionParamCallFlowFact {
	pass := csa.Pass
	tv := pass.TypesInfo.Types[sel.X]
	if !tv.IsValue() {
		return nil
	}

	ifaceType, ok := tv.Type.Underlying().(*types.Interface)
	if !ok {
		return nil
	}

	methodObj := pass.TypesInfo.Uses[sel.Sel]
	method, ok := methodObj.(*types.Func)
	if !ok {
		return nil
	}

	// Check for FunctionParamCallFlowFact directly on the interface method
	var callFlowFact facts.FunctionParamCallFlowFact
	if pass.ImportObjectFact(method, &callFlowFact) {
		return &callFlowFact
	}

	// Check global store for cross-package DI pattern
	if ifaceTypeName := findInterfaceTypeName(pass, ifaceType); ifaceTypeName != nil {
		key := facts.InterfaceMethodKey(ifaceTypeName.Pkg().Path(), ifaceTypeName.Name(), method.Name())
		if globalFact, ok := facts.LoadCallFlowFact(key); ok {
			// Key exists: return call flows if non-empty, otherwise nil
			// (empty means intersection eliminated all flows — not a miss)
			if len(globalFact.CallFlows) > 0 {
				return globalFact
			}
			return nil
		}
		// Key absent: mark global store miss for deferred re-analysis
		csa.hadGlobalStoreMiss = true
	}

	// Dynamically compute intersection from implementations
	implementingTypes := csa.InterfaceImpls.GetImplementingTypes(ifaceType)
	if len(implementingTypes) == 0 {
		return nil
	}

	var allCallFlowFacts []*facts.FunctionParamCallFlowFact
	for _, concreteType := range implementingTypes {
		concreteMethod := internal.FindMethodImplementation(concreteType, method)
		if concreteMethod == nil {
			continue
		}
		var cf facts.FunctionParamCallFlowFact
		if pass.ImportObjectFact(concreteMethod, &cf) {
			allCallFlowFacts = append(allCallFlowFacts, &cf)
		} else {
			allCallFlowFacts = append(allCallFlowFacts, nil)
		}
	}

	return facts.IntersectFunctionParamCallFlowFacts(allCallFlowFacts)
}

// flowInfo represents a parameter flow with index and wrapped flag.
// This interface abstracts the commonality between ParameterFlowInfo and FunctionParamCallFlowInfo.
type flowInfo interface {
	Index() int
	IsWrapped() bool
}

// paramFlowAdapter adapts facts.ParameterFlowInfo to flowInfo interface.
type paramFlowAdapter struct{ f facts.ParameterFlowInfo }

func (a paramFlowAdapter) Index() int      { return a.f.ParamIndex }
func (a paramFlowAdapter) IsWrapped() bool { return a.f.Wrapped }

// funcParamCallFlowAdapter adapts facts.FunctionParamCallFlowInfo to flowInfo interface.
type funcParamCallFlowAdapter struct{ f facts.FunctionParamCallFlowInfo }

func (a funcParamCallFlowAdapter) Index() int      { return a.f.ParamIndex }
func (a funcParamCallFlowAdapter) IsWrapped() bool { return a.f.Wrapped }

// resolveFlowErrors resolves errors from call arguments based on flow information.
func resolveFlowErrors(pass *analysis.Pass, call *ast.CallExpr, flows []flowInfo) []facts.ErrorInfo {
	var errs []facts.ErrorInfo

	for _, flow := range flows {
		if flow.Index() < 0 || flow.Index() >= len(call.Args) {
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
func resolveParameterFlowErrorsAST(pass *analysis.Pass, call *ast.CallExpr, flowFact *facts.ParameterFlowFact) []facts.ErrorInfo {
	flows := make([]flowInfo, len(flowFact.Flows))
	for i, f := range flowFact.Flows {
		flows[i] = paramFlowAdapter{f}
	}
	return resolveFlowErrors(pass, call, flows)
}

// extractErrorsFromExpr extracts known errors from an expression.
func extractErrorsFromExpr(pass *analysis.Pass, expr ast.Expr) []facts.ErrorInfo {
	var errs []facts.ErrorInfo

	switch e := expr.(type) {
	case *ast.Ident:
		obj := pass.TypesInfo.Uses[e]
		if varObj, ok := obj.(*types.Var); ok {
			var errorFact facts.ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				errs = append(errs, facts.ErrorInfo{
					PkgPath: errorFact.PkgPath,
					Name:    errorFact.Name,
					Wrapped: false,
				})
			} else if varObj.Pkg() != nil && internal.IsErrorType(varObj.Type()) {
				// Local error variable
				errs = append(errs, facts.ErrorInfo{
					PkgPath: varObj.Pkg().Path(),
					Name:    varObj.Name(),
					Wrapped: false,
				})
			}
			// Also check for FunctionErrorsFact (for closure variables)
			var fnFact facts.FunctionErrorsFact
			if pass.ImportObjectFact(varObj, &fnFact) {
				errs = append(errs, fnFact.Errors...)
			}
		}
		// Also check for named functions (e.g., passing namedFunc to a higher-order function)
		if funcObj, ok := obj.(*types.Func); ok {
			var fnFact facts.FunctionErrorsFact
			if pass.ImportObjectFact(funcObj, &fnFact) {
				errs = append(errs, fnFact.Errors...)
			}
		}

	case *ast.SelectorExpr:
		obj := pass.TypesInfo.Uses[e.Sel]
		if varObj, ok := obj.(*types.Var); ok {
			var errorFact facts.ErrorFact
			if pass.ImportObjectFact(varObj, &errorFact) {
				errs = append(errs, facts.ErrorInfo{
					PkgPath: errorFact.PkgPath,
					Name:    errorFact.Name,
					Wrapped: false,
				})
			}
		}
		// Also check for named functions (e.g., passing pkg.NamedFunc to a higher-order function)
		if funcObj, ok := obj.(*types.Func); ok {
			var fnFact facts.FunctionErrorsFact
			if pass.ImportObjectFact(funcObj, &fnFact) {
				errs = append(errs, fnFact.Errors...)
			}
		}

	case *ast.CallExpr:
		// If the argument is a function call, recursively get its errors
		calledFn := internal.GetCalledFunction(pass, e)
		if calledFn != nil {
			var fnFact facts.FunctionErrorsFact
			if pass.ImportObjectFact(calledFn, &fnFact) {
				errs = append(errs, fnFact.Errors...)
			}
			// Also check ParameterFlowFact for chained wrappers
			var flowFact facts.ParameterFlowFact
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
func extractErrorsFromCompositeLit(pass *analysis.Pass, compLit *ast.CompositeLit) []facts.ErrorInfo {
	var errs []facts.ErrorInfo

	tv := pass.TypesInfo.Types[compLit]
	if !tv.IsValue() {
		return nil
	}

	namedType := internal.ExtractNamedType(tv.Type)
	if namedType == nil {
		return nil
	}

	typeName := namedType.Obj()
	if typeName == nil {
		return nil
	}

	var errorFact facts.ErrorFact
	if pass.ImportObjectFact(typeName, &errorFact) {
		errs = append(errs, facts.ErrorInfo{
			PkgPath: errorFact.PkgPath,
			Name:    errorFact.Name,
			Wrapped: false,
		})
	} else if typeName.Pkg() != nil {
		// Local custom error type
		errs = append(errs, facts.ErrorInfo{
			PkgPath: typeName.Pkg().Path(),
			Name:    typeName.Name(),
			Wrapped: false,
		})
	}

	return errs
}

// analyzeFuncLitErrors analyzes a function literal (lambda) body to extract errors.
func analyzeFuncLitErrors(pass *analysis.Pass, funcLit *ast.FuncLit) []facts.ErrorInfo {
	tv := pass.TypesInfo.Types[funcLit]
	if !tv.IsValue() {
		return nil
	}

	sig, ok := tv.Type.(*types.Signature)
	if !ok {
		return nil
	}

	errorPositions := internal.FindErrorReturnPositions(sig)
	if len(errorPositions) == 0 {
		return nil
	}

	var errs []facts.ErrorInfo
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
func resolveFunctionParamCallFlowErrors(pass *analysis.Pass, call *ast.CallExpr, flowFact *facts.FunctionParamCallFlowFact) []facts.ErrorInfo {
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
			if internal.IsErrorType(results.At(i).Type()) {
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
				if internal.IsErrorType(varObj.Type()) {
					return varObj
				}
			}
		}
	}

	return nil
}
