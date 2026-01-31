package internal

import (
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// InterfaceImplementations caches interface -> implementing types mapping.
type InterfaceImplementations struct {
	// implementations maps interface types to their implementing named types
	implementations map[*types.Interface][]*types.Named
}

// NewInterfaceImplementations creates a new interface implementations cache.
func NewInterfaceImplementations() *InterfaceImplementations {
	return &InterfaceImplementations{
		implementations: make(map[*types.Interface][]*types.Named),
	}
}

// FindInterfaceImplementations discovers all interface implementations visible from the package.
// It scans named types in the current package and all directly imported packages,
// and checks if they implement any interfaces also visible from the package.
func FindInterfaceImplementations(pass *analysis.Pass) *InterfaceImplementations {
	impl := NewInterfaceImplementations()

	// Use inspector to ensure it's available (required dependency)
	_ = pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Collect interfaces from current package + imported packages
	var interfaces []*types.Interface
	collectInterfaces(pass.Pkg.Scope(), &interfaces)
	for _, imp := range pass.Pkg.Imports() {
		collectInterfaces(imp.Scope(), &interfaces)
	}

	if len(interfaces) == 0 {
		return impl
	}

	// Collect named types from current package + imported packages
	var namedTypes []*types.Named
	collectNamedTypes(pass.Pkg.Scope(), &namedTypes)
	for _, imp := range pass.Pkg.Imports() {
		collectNamedTypes(imp.Scope(), &namedTypes)
	}

	// For each interface, find implementing types
	for _, iface := range interfaces {
		for _, named := range namedTypes {
			if types.Implements(named, iface) {
				impl.implementations[iface] = append(impl.implementations[iface], named)
			} else if ptr := types.NewPointer(named); types.Implements(ptr, iface) {
				impl.implementations[iface] = append(impl.implementations[iface], named)
			}
		}
	}

	return impl
}

// collectInterfaces scans a scope and appends all interface types found.
func collectInterfaces(scope *types.Scope, interfaces *[]*types.Interface) {
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		typeName, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		if iface, ok := typeName.Type().Underlying().(*types.Interface); ok {
			*interfaces = append(*interfaces, iface)
		}
	}
}

// collectNamedTypes scans a scope and appends all non-interface named types found.
func collectNamedTypes(scope *types.Scope, namedTypes *[]*types.Named) {
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		typeName, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		if named, ok := typeName.Type().(*types.Named); ok {
			// Skip interface types themselves
			if _, isIface := named.Underlying().(*types.Interface); isIface {
				continue
			}
			*namedTypes = append(*namedTypes, named)
		}
	}
}

// GetImplementingTypes returns all types that implement the given interface.
func (impl *InterfaceImplementations) GetImplementingTypes(iface *types.Interface) []*types.Named {
	return impl.implementations[iface]
}

// FindMethodImplementation finds the concrete method on a type that implements
// an interface method.
func FindMethodImplementation(concreteType *types.Named, ifaceMethod *types.Func) *types.Func {
	methodName := ifaceMethod.Name()

	// Check methods on the named type itself
	for i := 0; i < concreteType.NumMethods(); i++ {
		method := concreteType.Method(i)
		if method.Name() == methodName {
			return method
		}
	}

	// Check methods on the pointer type
	ptrType := types.NewPointer(concreteType)
	methodSet := types.NewMethodSet(ptrType)
	sel := methodSet.Lookup(nil, methodName)
	if sel != nil {
		if fn, ok := sel.Obj().(*types.Func); ok {
			return fn
		}
	}

	return nil
}

// GetInterfaceType extracts the interface type from a type, handling pointers.
func GetInterfaceType(t types.Type) *types.Interface {
	switch typ := t.(type) {
	case *types.Interface:
		return typ
	case *types.Named:
		if iface, ok := typ.Underlying().(*types.Interface); ok {
			return iface
		}
	case *types.Pointer:
		return GetInterfaceType(typ.Elem())
	}
	return nil
}
