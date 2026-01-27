package goexhauerrors

import (
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// interfaceImplementations caches interface -> implementing types mapping.
type interfaceImplementations struct {
	// implementations maps interface types to their implementing named types
	implementations map[*types.Interface][]*types.Named
}

// newInterfaceImplementations creates a new interface implementations cache.
func newInterfaceImplementations() *interfaceImplementations {
	return &interfaceImplementations{
		implementations: make(map[*types.Interface][]*types.Named),
	}
}

// findInterfaceImplementations discovers all interface implementations in the package.
// It scans all named types and checks if they implement any interfaces defined in the package.
func findInterfaceImplementations(pass *analysis.Pass) *interfaceImplementations {
	impl := newInterfaceImplementations()

	// Collect all interfaces defined in the package
	var interfaces []*types.Interface
	scope := pass.Pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		typeName, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		if iface, ok := typeName.Type().Underlying().(*types.Interface); ok {
			interfaces = append(interfaces, iface)
		}
	}

	if len(interfaces) == 0 {
		return impl
	}

	// Collect all named types in the package
	var namedTypes []*types.Named
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
			namedTypes = append(namedTypes, named)
		}
	}

	// For each interface, find implementing types
	for _, iface := range interfaces {
		for _, named := range namedTypes {
			// Check if T implements the interface
			if types.Implements(named, iface) {
				impl.implementations[iface] = append(impl.implementations[iface], named)
			} else if ptr := types.NewPointer(named); types.Implements(ptr, iface) {
				// Check if *T implements the interface
				impl.implementations[iface] = append(impl.implementations[iface], named)
			}
		}
	}

	return impl
}

// getImplementingTypes returns all types that implement the given interface.
func (impl *interfaceImplementations) getImplementingTypes(iface *types.Interface) []*types.Named {
	return impl.implementations[iface]
}

// findMethodImplementation finds the concrete method on a type that implements
// an interface method.
func findMethodImplementation(concreteType *types.Named, ifaceMethod *types.Func) *types.Func {
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

// getInterfaceType extracts the interface type from a type, handling pointers.
func getInterfaceType(t types.Type) *types.Interface {
	switch typ := t.(type) {
	case *types.Interface:
		return typ
	case *types.Named:
		if iface, ok := typ.Underlying().(*types.Interface); ok {
			return iface
		}
	case *types.Pointer:
		return getInterfaceType(typ.Elem())
	}
	return nil
}
