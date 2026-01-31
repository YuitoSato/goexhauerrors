package internal

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"testing"
)

func TestIsErrorType(t *testing.T) {
	errorType := types.Universe.Lookup("error").Type()
	errorInterface := errorType.Underlying().(*types.Interface)

	// Create a non-error interface (e.g., interface{ Foo() })
	fooSig := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	fooFunc := types.NewFunc(token.NoPos, nil, "Foo", fooSig)
	nonErrorIface := types.NewInterfaceType([]*types.Func{fooFunc}, nil)
	nonErrorIface.Complete()

	// Create a named type wrapping the error interface
	pkg := types.NewPackage("example.com/pkg", "pkg")
	namedError := types.NewNamed(
		types.NewTypeName(token.NoPos, pkg, "MyError", nil),
		errorInterface,
		nil,
	)

	tests := []struct {
		name string
		typ  types.Type
		want bool
	}{
		{"builtin error", errorType, true},
		{"non-error interface", nonErrorIface, false},
		{"named type wrapping error", namedError, true},
		{"basic int", types.Typ[types.Int], false},
		{"basic string", types.Typ[types.String], false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsErrorType(tt.typ)
			if got != tt.want {
				t.Errorf("IsErrorType(%v) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}

func TestExtractStringLiteral(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expr
		want string
	}{
		{
			"double-quoted string",
			&ast.BasicLit{Kind: token.STRING, Value: `"hello world"`},
			"hello world",
		},
		{
			"raw backtick string",
			&ast.BasicLit{Kind: token.STRING, Value: "`raw string`"},
			"raw string",
		},
		{
			"non-string literal INT",
			&ast.BasicLit{Kind: token.INT, Value: "42"},
			"",
		},
		{
			"non-BasicLit expression",
			&ast.Ident{Name: "x"},
			"",
		},
		{
			"single character string",
			&ast.BasicLit{Kind: token.STRING, Value: `"a"`},
			"a",
		},
		{
			"empty string literal",
			&ast.BasicLit{Kind: token.STRING, Value: `""`},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractStringLiteral(tt.expr)
			if got != tt.want {
				t.Errorf("ExtractStringLiteral() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindErrorReturnPositions(t *testing.T) {
	errorType := types.Universe.Lookup("error").Type()
	intType := types.Typ[types.Int]

	tests := []struct {
		name    string
		results []*types.Var
		want    []int
	}{
		{"no return values", nil, nil},
		{"single error return", []*types.Var{
			types.NewVar(token.NoPos, nil, "", errorType),
		}, []int{0}},
		{"int and error", []*types.Var{
			types.NewVar(token.NoPos, nil, "", intType),
			types.NewVar(token.NoPos, nil, "", errorType),
		}, []int{1}},
		{"error and error", []*types.Var{
			types.NewVar(token.NoPos, nil, "", errorType),
			types.NewVar(token.NoPos, nil, "", errorType),
		}, []int{0, 1}},
		{"no error returns", []*types.Var{
			types.NewVar(token.NoPos, nil, "", intType),
			types.NewVar(token.NoPos, nil, "", types.Typ[types.String]),
		}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultsTuple := types.NewTuple(tt.results...)
			sig := types.NewSignatureType(nil, nil, nil, nil, resultsTuple, false)
			got := FindErrorReturnPositions(sig)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindErrorReturnPositions() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindErrorParamVars(t *testing.T) {
	errorType := types.Universe.Lookup("error").Type()
	intType := types.Typ[types.Int]

	tests := []struct {
		name   string
		params []*types.Var
		want   map[int]bool // indices that should be in the result
	}{
		{"no params", nil, map[int]bool{}},
		{"single error param", []*types.Var{
			types.NewVar(token.NoPos, nil, "err", errorType),
		}, map[int]bool{0: true}},
		{"multiple params some error", []*types.Var{
			types.NewVar(token.NoPos, nil, "n", intType),
			types.NewVar(token.NoPos, nil, "err1", errorType),
			types.NewVar(token.NoPos, nil, "s", types.Typ[types.String]),
			types.NewVar(token.NoPos, nil, "err2", errorType),
		}, map[int]bool{1: true, 3: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paramsTuple := types.NewTuple(tt.params...)
			sig := types.NewSignatureType(nil, nil, nil, paramsTuple, nil, false)
			got := FindErrorParamVars(sig)

			if len(got) != len(tt.want) {
				t.Fatalf("FindErrorParamVars() returned %d entries, want %d", len(got), len(tt.want))
			}

			for v, idx := range got {
				if !tt.want[idx] {
					t.Errorf("unexpected param %q at index %d", v.Name(), idx)
				}
			}
		})
	}
}

func TestExtractNamedType(t *testing.T) {
	pkg := types.NewPackage("example.com/pkg", "pkg")
	named := types.NewNamed(
		types.NewTypeName(token.NoPos, pkg, "MyType", nil),
		types.Typ[types.Int],
		nil,
	)
	ptrToNamed := types.NewPointer(named)
	ptrToInt := types.NewPointer(types.Typ[types.Int])

	tests := []struct {
		name    string
		typ     types.Type
		wantNil bool
		wantObj string // expected type name if not nil
	}{
		{"named type", named, false, "MyType"},
		{"pointer to named type", ptrToNamed, false, "MyType"},
		{"pointer to non-named", ptrToInt, true, ""},
		{"non-named non-pointer", types.Typ[types.Int], true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractNamedType(tt.typ)
			if tt.wantNil {
				if got != nil {
					t.Errorf("ExtractNamedType() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Fatal("ExtractNamedType() = nil, want non-nil")
				}
				if got.Obj().Name() != tt.wantObj {
					t.Errorf("ExtractNamedType().Obj().Name() = %q, want %q", got.Obj().Name(), tt.wantObj)
				}
			}
		})
	}
}

func TestExtractCompositeLit(t *testing.T) {
	compLit := &ast.CompositeLit{
		Type: &ast.Ident{Name: "MyError"},
	}

	tests := []struct {
		name    string
		call    *ast.CallExpr
		wantNil bool
	}{
		{
			"CallExpr with &CompositeLit as Fun",
			&ast.CallExpr{
				Fun: &ast.UnaryExpr{
					Op: token.AND,
					X:  compLit,
				},
			},
			false,
		},
		{
			"CallExpr with regular Ident Fun",
			&ast.CallExpr{
				Fun: &ast.Ident{Name: "foo"},
			},
			true,
		},
		{
			"CallExpr with non-& UnaryExpr",
			&ast.CallExpr{
				Fun: &ast.UnaryExpr{
					Op: token.SUB,
					X:  compLit,
				},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractCompositeLit(tt.call)
			if tt.wantNil {
				if got != nil {
					t.Errorf("ExtractCompositeLit() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Fatal("ExtractCompositeLit() = nil, want non-nil")
				}
				if got != compLit {
					t.Errorf("ExtractCompositeLit() returned different CompositeLit")
				}
			}
		})
	}
}

func TestFindWrapVerbIndices(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   []int
	}{
		{"no verbs", "hello", nil},
		{"single %w", "%w", []int{0}},
		{"%s %w", "%s %w", []int{1}},
		{"%w %s %w", "%w %s %w", []int{0, 2}},
		{"%% escaped", "%%", nil},
		{"%%w not a wrap", "%%w", nil},
		{"%+v %w flags before verb", "%+v %w", []int{1}},
		{"%.2f %w precision", "%.2f %w", []int{1}},
		{"%10.2f %w width+precision", "%10.2f %w", []int{1}},
		{"empty string", "", nil},
		{"%w%w consecutive", "%w%w", []int{0, 1}},
		{"incomplete format at end", "%", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindWrapVerbIndices(tt.format)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FindWrapVerbIndices(%q) = %v, want %v", tt.format, got, tt.want)
			}
		})
	}
}

func TestSplitErrorKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want []string
	}{
		{"pkg.Name", "pkg.Name", []string{"pkg", "Name"}},
		{"a/b/c.Name", "a/b/c.Name", []string{"a/b/c", "Name"}},
		{"noperiod", "noperiod", nil},
		{"empty string", "", nil},
		{".Name", ".Name", []string{"", "Name"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitErrorKey(tt.key)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitErrorKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
