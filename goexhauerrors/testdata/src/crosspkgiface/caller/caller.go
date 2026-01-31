package caller

import (
	"errors"

	"crosspkgiface/iface"
	_ "crosspkgiface/impl" // import for implementation discovery
)

// =============================================================================
// Test 1: Cross-package basic interface call — warns for all implementations' errors
// =============================================================================

func TestCrossPkgInterfaceCallBad(s iface.Service) {
	err := s.DoWork() // want "missing errors.Is check for crosspkgiface/iface.ErrIfaceA" "missing errors.Is check for crosspkgiface/iface.ErrIfaceB"
	if err != nil {
		println(err.Error())
	}
}

func TestCrossPkgInterfaceCallPartialCheck(s iface.Service) {
	err := s.DoWork() // want "missing errors.Is check for crosspkgiface/iface.ErrIfaceB"
	if errors.Is(err, iface.ErrIfaceA) {
		println("error a")
	}
}

func TestCrossPkgInterfaceCallGood(s iface.Service) {
	err := s.DoWork()
	if errors.Is(err, iface.ErrIfaceA) {
		println("error a")
	} else if errors.Is(err, iface.ErrIfaceB) {
		println("error b")
	}
}

// =============================================================================
// Test 2: Cross-package interface + ParameterFlowFact — propagation
// =============================================================================

func TestCrossPkgTransformPropagateGood(t iface.Transformer) error { // want TestCrossPkgTransformPropagateGood:`\[crosspkgiface/iface.ErrIfaceA, crosspkgiface/iface.ErrIfaceB, crosspkgiface/iface.ErrIfaceC\]` TestCrossPkgTransformPropagateGood:`\[call:0\]`
	err := iface.GetAllErrors()
	return t.Transform(err) // OK - all impls propagate
}

// =============================================================================
// Test 3: Cross-package interface + ParameterCheckedErrorsFact
// =============================================================================

func TestCrossPkgMapperPartial(m iface.Mapper) error { // want TestCrossPkgMapperPartial:`\[call:0\]`
	err := iface.GetAllErrors() // want "missing errors.Is check for crosspkgiface/iface.ErrIfaceB" "missing errors.Is check for crosspkgiface/iface.ErrIfaceC"
	return m.Map(err) // ErrIfaceA is checked inside all impls
}

// =============================================================================
// Test 4: Nested propagation — function returning interface method result
// =============================================================================

func intermediateFunc(s iface.Service) error { // want intermediateFunc:`\[crosspkgiface/iface.ErrIfaceA, crosspkgiface/iface.ErrIfaceB\]` intermediateFunc:`\[call:0\]`
	err := s.DoWork()
	return err
}

func TestNestedCrossPkgInterfaceBad(s iface.Service) {
	err := intermediateFunc(s) // want "missing errors.Is check for crosspkgiface/iface.ErrIfaceA" "missing errors.Is check for crosspkgiface/iface.ErrIfaceB"
	if err != nil {
		println(err.Error())
	}
}

func TestNestedCrossPkgInterfaceGood(s iface.Service) {
	err := intermediateFunc(s)
	if errors.Is(err, iface.ErrIfaceA) {
		println("error a")
	} else if errors.Is(err, iface.ErrIfaceB) {
		println("error b")
	}
}
