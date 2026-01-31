package internal

import (
	"go/token"
	"go/types"
	"testing"
)

func TestGetInterfaceType(t *testing.T) {
	// Create a simple interface: interface{ Foo() }
	fooSig := types.NewSignatureType(nil, nil, nil, nil, nil, false)
	fooFunc := types.NewFunc(token.NoPos, nil, "Foo", fooSig)
	iface := types.NewInterfaceType([]*types.Func{fooFunc}, nil)
	iface.Complete()

	// Named type with interface underlying
	pkg := types.NewPackage("example.com/pkg", "pkg")
	namedIface := types.NewNamed(
		types.NewTypeName(token.NoPos, pkg, "MyInterface", nil),
		iface,
		nil,
	)

	// Pointer to named interface
	ptrToNamedIface := types.NewPointer(namedIface)

	// Nested pointer to named interface
	nestedPtrToNamedIface := types.NewPointer(ptrToNamedIface)

	// Non-interface type
	namedStruct := types.NewNamed(
		types.NewTypeName(token.NoPos, pkg, "MyStruct", nil),
		types.NewStruct(nil, nil),
		nil,
	)

	tests := []struct {
		name    string
		typ     types.Type
		wantNil bool
	}{
		{"interface type directly", iface, false},
		{"named type with interface underlying", namedIface, false},
		{"pointer to named interface", ptrToNamedIface, false},
		{"non-interface type", namedStruct, true},
		{"nested pointer to named interface", nestedPtrToNamedIface, false},
		{"basic int type", types.Typ[types.Int], true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetInterfaceType(tt.typ)
			if tt.wantNil {
				if got != nil {
					t.Errorf("GetInterfaceType() = %v, want nil", got)
				}
			} else {
				if got == nil {
					t.Fatal("GetInterfaceType() = nil, want non-nil")
				}
			}
		})
	}
}

func TestFindMethodImplementation(t *testing.T) {
	pkg := types.NewPackage("example.com/pkg", "pkg")

	// Create an interface method signature: Error() string
	errorMethodSig := types.NewSignatureType(
		nil, nil, nil,
		nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String])),
		false,
	)
	ifaceMethod := types.NewFunc(token.NoPos, pkg, "Error", errorMethodSig)

	// Create a different interface method: Other() string
	otherMethod := types.NewFunc(token.NoPos, pkg, "Other", errorMethodSig)

	// Create a concrete named type with the Error method (value receiver)
	concreteTypeName := types.NewTypeName(token.NoPos, pkg, "ConcreteError", nil)
	concreteType := types.NewNamed(concreteTypeName, types.NewStruct(nil, nil), nil)

	// Add Error method with value receiver
	recv := types.NewVar(token.NoPos, pkg, "e", concreteType)
	methodSig := types.NewSignatureType(
		recv, nil, nil,
		nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String])),
		false,
	)
	concreteMethod := types.NewFunc(token.NoPos, pkg, "Error", methodSig)
	concreteType.AddMethod(concreteMethod)

	// Create a type with pointer receiver method only
	ptrTypeName := types.NewTypeName(token.NoPos, pkg, "PtrError", nil)
	ptrType := types.NewNamed(ptrTypeName, types.NewStruct(nil, nil), nil)

	ptrRecv := types.NewVar(token.NoPos, pkg, "e", types.NewPointer(ptrType))
	ptrMethodSig := types.NewSignatureType(
		ptrRecv, nil, nil,
		nil,
		types.NewTuple(types.NewVar(token.NoPos, nil, "", types.Typ[types.String])),
		false,
	)
	ptrMethod := types.NewFunc(token.NoPos, pkg, "Error", ptrMethodSig)
	ptrType.AddMethod(ptrMethod)

	t.Run("method found on value receiver", func(t *testing.T) {
		got := FindMethodImplementation(concreteType, ifaceMethod)
		if got == nil {
			t.Fatal("FindMethodImplementation() = nil, want non-nil")
		}
		if got.Name() != "Error" {
			t.Errorf("FindMethodImplementation().Name() = %q, want %q", got.Name(), "Error")
		}
	})

	t.Run("missing method", func(t *testing.T) {
		got := FindMethodImplementation(concreteType, otherMethod)
		if got != nil {
			t.Errorf("FindMethodImplementation() = %v, want nil", got)
		}
	})

	t.Run("method on pointer receiver found via MethodSet", func(t *testing.T) {
		got := FindMethodImplementation(ptrType, ifaceMethod)
		if got == nil {
			t.Fatal("FindMethodImplementation() = nil, want non-nil")
		}
		if got.Name() != "Error" {
			t.Errorf("FindMethodImplementation().Name() = %q, want %q", got.Name(), "Error")
		}
	})
}
