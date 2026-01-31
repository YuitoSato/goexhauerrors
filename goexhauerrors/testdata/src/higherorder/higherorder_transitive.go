package higherorder

import (
	"errors"
	"fmt"
)

// =============================================================================
// Test: Transitive FunctionParamCallFlowFact - Wrapper passes fn to another
// higher-order function instead of calling it directly
// =============================================================================

// TransitiveWrapper passes fn to RunWithCallback instead of calling fn() directly
func TransitiveWrapper(fn func() error) error { // want TransitiveWrapper:`\[call:0\]`
	return RunWithCallback(fn)
}

func TestTransitiveWrapperLambda() {
	err := TransitiveWrapper(func() error { // want "missing errors.Is check for higherorder.ErrNotFound"
		return ErrNotFound
	})
	if err != nil {
		println(err.Error())
	}
}

func TestTransitiveWrapperLambdaGood() {
	err := TransitiveWrapper(func() error {
		return ErrNotFound
	})
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test: Transitive with named function
// =============================================================================

func TestTransitiveWrapperNamedFunc() {
	err := TransitiveWrapper(namedCallbackFunc) // want "missing errors.Is check for higherorder.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func TestTransitiveWrapperNamedFuncGood() {
	err := TransitiveWrapper(namedCallbackFunc)
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test: Double transitive - wrapper of a wrapper of a higher-order function
// =============================================================================

func DoubleTransitiveWrapper(fn func() error) error { // want DoubleTransitiveWrapper:`\[call:0\]`
	return TransitiveWrapper(fn)
}

func TestDoubleTransitiveWrapper() {
	err := DoubleTransitiveWrapper(func() error { // want "missing errors.Is check for higherorder.ErrNotFound"
		return ErrNotFound
	})
	if err != nil {
		println(err.Error())
	}
}

func TestDoubleTransitiveWrapperGood() {
	err := DoubleTransitiveWrapper(func() error {
		return ErrNotFound
	})
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test: Transitive with method (RunInTx is a method with receiver)
// =============================================================================

func TransitiveMethodWrapper(fn func(tx Tx) error) error { // want TransitiveMethodWrapper:`\[call:0\]`
	conn := &Conn{}
	return conn.RunInTx(nil, fn)
}

func TestTransitiveMethodWrapper() {
	err := TransitiveMethodWrapper(func(tx Tx) error { // want "missing errors.Is check for higherorder.ErrNotFound"
		return ErrNotFound
	})
	if err != nil {
		println(err.Error())
	}
}

func TestTransitiveMethodWrapperGood() {
	err := TransitiveMethodWrapper(func(tx Tx) error {
		return ErrNotFound
	})
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test: Transitive wrapper with fmt.Errorf wrapping
// Exercises: analyzeErrorfWrappingForFunctionParamCalls → recursive
// traceValueToFunctionParamCalls → traceTransitiveFunctionParamCallFlow
// =============================================================================

func TransitiveWrapperWithErrorf(fn func() error) error { // want TransitiveWrapperWithErrorf:`\[wrapped:call:0\]`
	return fmt.Errorf("wrapped: %w", RunWithCallback(fn))
}

func TestTransitiveWrapperWithErrorf() {
	err := TransitiveWrapperWithErrorf(func() error { // want "missing errors.Is check for higherorder.ErrNotFound"
		return ErrNotFound
	})
	if err != nil {
		println(err.Error())
	}
}

func TestTransitiveWrapperWithErrorfGood() {
	err := TransitiveWrapperWithErrorf(func() error {
		return ErrNotFound
	})
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test: Transitive wrapper that also returns its own errors (mixed case)
// Exercises: FunctionErrorsFact and FunctionParamCallFlowFact coexistence
// =============================================================================

var ErrMixed = errors.New("mixed error") // want ErrMixed:`higherorder.ErrMixed`

func TransitiveWrapperWithOwnErrors(fn func() error) error { // want TransitiveWrapperWithOwnErrors:`\[call:0\]` TransitiveWrapperWithOwnErrors:`\[higherorder.ErrMixed\]`
	if fn == nil {
		return ErrMixed
	}
	return RunWithCallback(fn)
}

func TestTransitiveWrapperWithOwnErrorsBad() {
	err := TransitiveWrapperWithOwnErrors(func() error { // want "missing errors.Is check for higherorder.ErrMixed" "missing errors.Is check for higherorder.ErrNotFound"
		return ErrNotFound
	})
	if err != nil {
		println(err.Error())
	}
}

func TestTransitiveWrapperWithOwnErrorsGood() {
	err := TransitiveWrapperWithOwnErrors(func() error {
		return ErrNotFound
	})
	if errors.Is(err, ErrMixed) {
		println("mixed")
	} else if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}
