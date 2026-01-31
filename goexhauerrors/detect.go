package goexhauerrors

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// detectLocalErrors finds local errors in the current package and exports facts.
// It detects:
// 1. var Err* = errors.New("...") pattern (sentinel errors)
// 2. Custom error types (structs implementing error interface)
func detectLocalErrors(pass *analysis.Pass) *localErrors {
	result := newLocalErrors()

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Detect sentinel error variables
	detectSentinelVars(pass, insp, result)

	// Detect custom error types
	detectCustomErrorTypes(pass, result)

	return result
}

// detectSentinelVars finds var Err* = errors.New("...") patterns.
func detectSentinelVars(pass *analysis.Pass, insp *inspector.Inspector, result *localErrors) {
	nodeFilter := []ast.Node{
		(*ast.GenDecl)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		genDecl := n.(*ast.GenDecl)
		if genDecl.Tok != token.VAR {
			return
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for i, name := range valueSpec.Names {
				// Get the object from type info
				obj := pass.TypesInfo.Defs[name]
				if obj == nil {
					continue
				}

				varObj, ok := obj.(*types.Var)
				if !ok {
					continue
				}

				// Check if type is error
				if !isErrorType(varObj.Type()) {
					continue
				}

				// Check initialization pattern
				if i < len(valueSpec.Values) {
					if isSentinelInit(pass, valueSpec.Values[i]) {
						result.vars[varObj] = true
						// Only export fact for exported errors (cross-package analysis)
						if token.IsExported(name.Name) {
							fact := &ErrorFact{
								Name:    name.Name,
								PkgPath: pass.Pkg.Path(),
							}
							pass.ExportObjectFact(varObj, fact)
						}
					}
				}
			}
		}
	})
}

// detectCustomErrorTypes finds struct types that implement the error interface.
func detectCustomErrorTypes(pass *analysis.Pass, result *localErrors) {
	errorInterface := types.Universe.Lookup("error").Type().Underlying().(*types.Interface)

	scope := pass.Pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		typeName, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}

		// Check if this type implements error interface
		namedType, ok := typeName.Type().(*types.Named)
		if !ok {
			continue
		}

		// Check both pointer and value receiver implementations
		ptrType := types.NewPointer(namedType)
		if types.Implements(namedType, errorInterface) || types.Implements(ptrType, errorInterface) {
			// Skip if it's the error interface itself
			if typeName.Name() == "error" {
				continue
			}

			result.types[typeName] = true
			// Only export fact for exported types (cross-package analysis)
			if token.IsExported(name) {
				fact := &ErrorFact{
					Name:    typeName.Name(),
					PkgPath: pass.Pkg.Path(),
				}
				pass.ExportObjectFact(typeName, fact)
			}
		}
	}
}

// isErrorType checks if the given type is the error interface.
func isErrorType(t types.Type) bool {
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

// isSentinelInit checks if the expression is a sentinel error initialization.
// Supported patterns:
// - errors.New("...")
// - fmt.Errorf("...") without %w
func isSentinelInit(pass *analysis.Pass, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}

	// Check for errors.New("...")
	if isErrorsNewCall(pass, call) {
		return true
	}

	// Check for fmt.Errorf("...") without %w
	if isFmtErrorfWithoutWrap(pass, call) {
		return true
	}

	return false
}

// isErrorsNewCall checks if the call is errors.New().
func isErrorsNewCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	if sel.Sel.Name != "New" {
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

// isFmtErrorfWithoutWrap checks if the call is fmt.Errorf without %w.
func isFmtErrorfWithoutWrap(pass *analysis.Pass, call *ast.CallExpr) bool {
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

	if pkgName.Imported().Path() != "fmt" {
		return false
	}

	// Check format string for %w
	if len(call.Args) < 1 {
		return true
	}

	formatStr := extractStringLiteral(call.Args[0])
	if formatStr == "" {
		return true // Can't determine, assume no %w
	}

	return !strings.Contains(formatStr, "%w")
}

// extractStringLiteral extracts the string value from a basic literal.
func extractStringLiteral(expr ast.Expr) string {
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
