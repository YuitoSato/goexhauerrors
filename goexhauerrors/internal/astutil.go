package internal

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/YuitoSato/goexhauerrors/goexhauerrors/facts"
	"golang.org/x/tools/go/analysis"
)

// IsErrorType checks if the given type is the error interface.
func IsErrorType(t types.Type) bool {
	// Check if it's exactly the error interface
	if named, ok := t.(*types.Named); ok {
		if named.Obj().Pkg() == nil && named.Obj().Name() == "error" {
			return true
		}
	}
	// Check if it's an interface that matches error
	if iface, ok := t.Underlying().(*types.Interface); ok {
		errorInterface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
		return types.Identical(iface, errorInterface)
	}
	return false
}

// ExtractStringLiteral extracts the string value from a basic literal.
func ExtractStringLiteral(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	// Remove quotes
	s := lit.Value
	if len(s) >= 2 {
		if s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1]
		}
		if s[0] == '`' && s[len(s)-1] == '`' {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// FindErrorReturnPositions finds which return value positions are error type.
func FindErrorReturnPositions(sig *types.Signature) []int {
	var positions []int
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		if IsErrorType(results.At(i).Type()) {
			positions = append(positions, i)
		}
	}
	return positions
}

// FindErrorParamVars returns a map from *types.Var (parameter) to its index
// for all error-typed parameters in the signature.
func FindErrorParamVars(sig *types.Signature) map[*types.Var]int {
	result := make(map[*types.Var]int)
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		param := params.At(i)
		if IsErrorType(param.Type()) {
			result[param] = i
		}
	}
	return result
}

// GetCalledFunction returns the *types.Func for a call expression if available.
func GetCalledFunction(pass *analysis.Pass, call *ast.CallExpr) *types.Func {
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

// ExtractCompositeLit extracts composite literal from &MyError{} pattern.
func ExtractCompositeLit(call *ast.CallExpr) *ast.CompositeLit {
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

// ExtractNamedType extracts the named type from a type, handling pointers.
func ExtractNamedType(t types.Type) *types.Named {
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

// IsFmtErrorfCall checks if the call is fmt.Errorf.
func IsFmtErrorfCall(pass *analysis.Pass, call *ast.CallExpr) bool {
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

// FindWrapVerbIndices finds the argument indices for %w verbs in format string.
func FindWrapVerbIndices(format string) []int {
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

// SplitErrorKey splits "pkg.Name" into [pkg, Name].
func SplitErrorKey(key string) []string {
	lastDot := strings.LastIndex(key, ".")
	if lastDot < 0 {
		return nil
	}
	return []string{key[:lastDot], key[lastDot+1:]}
}

// ExtractErrorInfoFromExpr extracts ErrorInfo from an errors.Is target expression.
func ExtractErrorInfoFromExpr(pass *analysis.Pass, expr ast.Expr) *facts.ErrorInfo {
	key := ExtractErrorKey(pass, expr)
	if key == "" {
		return nil
	}
	parts := SplitErrorKey(key)
	if parts == nil {
		return nil
	}
	return &facts.ErrorInfo{PkgPath: parts[0], Name: parts[1]}
}

// ExtractErrorInfoFromAsTarget extracts ErrorInfo from an errors.As target expression.
func ExtractErrorInfoFromAsTarget(pass *analysis.Pass, expr ast.Expr) *facts.ErrorInfo {
	key := ExtractErrorKeyFromAsTarget(pass, expr)
	if key == "" {
		return nil
	}
	parts := SplitErrorKey(key)
	if parts == nil {
		return nil
	}
	return &facts.ErrorInfo{PkgPath: parts[0], Name: parts[1]}
}

// IsErrorsIsCall checks if the call is errors.Is().
func IsErrorsIsCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	return IsErrorsPkgCall(pass, call, "Is")
}

// IsErrorsAsCall checks if the call is errors.As().
func IsErrorsAsCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	return IsErrorsPkgCall(pass, call, "As")
}

// IsErrorsPkgCall checks if the call is errors.<funcName>().
func IsErrorsPkgCall(pass *analysis.Pass, call *ast.CallExpr, funcName string) bool {
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

// ExtractErrorKey extracts the error key from an errors.Is second argument.
func ExtractErrorKey(pass *analysis.Pass, expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		obj := pass.TypesInfo.Uses[e]
		if varObj, ok := obj.(*types.Var); ok {
			var errorFact facts.ErrorFact
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
			var errorFact facts.ErrorFact
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

// ExtractErrorKeyFromAsTarget extracts the error key from errors.As target.
// errors.As(err, &target) where target is *SomeErrorType
func ExtractErrorKeyFromAsTarget(pass *analysis.Pass, expr ast.Expr) string {
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
			var errorFact facts.ErrorFact
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
		var errorFact facts.ErrorFact
		if pass.ImportObjectFact(typeName, &errorFact) {
			return errorFact.PkgPath + "." + errorFact.Name
		}
		if typeName.Pkg() != nil {
			return typeName.Pkg().Path() + "." + typeName.Name()
		}
	}

	return ""
}

// ExtractTypeNameFromExpr extracts the error key from a type expression in a type switch case.
// Handles: *SomeError, SomeError, *pkg.SomeError, pkg.SomeError
func ExtractTypeNameFromExpr(pass *analysis.Pass, expr ast.Expr) string {
	// Handle pointer types: *SomeError
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	tv := pass.TypesInfo.Types[expr]
	if !tv.IsType() {
		return ""
	}

	t := tv.Type
	// Unwrap pointer if still present
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}

	if named, ok := t.(*types.Named); ok {
		typeName := named.Obj()
		var errorFact facts.ErrorFact
		if pass.ImportObjectFact(typeName, &errorFact) {
			return errorFact.PkgPath + "." + errorFact.Name
		}
		if typeName.Pkg() != nil {
			return typeName.Pkg().Path() + "." + typeName.Name()
		}
	}
	return ""
}

// ReferencesVariable checks if an expression references the given variable.
func ReferencesVariable(pass *analysis.Pass, expr ast.Expr, targetVar *types.Var) bool {
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

// FindArgIndexForVar finds the index of a variable in a call's arguments.
// Returns -1 if the variable is not found as a direct argument.
func FindArgIndexForVar(pass *analysis.Pass, call *ast.CallExpr, targetVar *types.Var) int {
	for i, arg := range call.Args {
		if ident, ok := arg.(*ast.Ident); ok {
			obj := pass.TypesInfo.Uses[ident]
			if obj == targetVar {
				return i
			}
		}
	}
	return -1
}

// IsFmtErrorfWrappingVariable checks if a fmt.Errorf call wraps the given variable with %w.
func IsFmtErrorfWrappingVariable(pass *analysis.Pass, call *ast.CallExpr, targetVar *types.Var) bool {
	if len(call.Args) < 1 {
		return false
	}

	formatStr := ExtractStringLiteral(call.Args[0])
	if formatStr == "" {
		return false
	}

	wrapIndices := FindWrapVerbIndices(formatStr)
	for _, wrapIdx := range wrapIndices {
		argIdx := 1 + wrapIdx // args[0] is format string
		if argIdx < len(call.Args) {
			if ident, ok := call.Args[argIdx].(*ast.Ident); ok {
				obj := pass.TypesInfo.Uses[ident]
				if obj == targetVar {
					return true
				}
			}
		}
	}
	return false
}
