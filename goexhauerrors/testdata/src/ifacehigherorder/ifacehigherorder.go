package ifacehigherorder

import "errors"

var ErrNotFound = errors.New("not found")   // want ErrNotFound:`ifacehigherorder.ErrNotFound`
var ErrUpdate = errors.New("update failed") // want ErrUpdate:`ifacehigherorder.ErrUpdate`

// =============================================================================
// Interface with higher-order function pattern
// =============================================================================

type TxRunner interface {
	RunInTx(fn func() error) error // want RunInTx:`\[call:0\]`
}

type DBRunner struct{}

func (d *DBRunner) RunInTx(fn func() error) error { // want RunInTx:`\[call:0\]`
	return fn()
}

// =============================================================================
// Test 1: Interface method call with lambda - should detect unchecked errors
// =============================================================================

func TestInterfaceHigherOrder(r TxRunner) {
	err := r.RunInTx(func() error { // want "missing errors.Is check for ifacehigherorder.ErrNotFound"
		return ErrNotFound
	})
	if err != nil {
		println(err.Error())
	}
}

// =============================================================================
// Test 2: Interface method call with lambda - properly checked (no warning)
// =============================================================================

func TestInterfaceHigherOrderGood(r TxRunner) {
	err := r.RunInTx(func() error {
		return ErrNotFound
	})
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test 3: Interface method call with multiple errors in lambda
// =============================================================================

func TestInterfaceHigherOrderMultiple(r TxRunner) {
	err := r.RunInTx(func() error { // want "missing errors.Is check for ifacehigherorder.ErrNotFound" "missing errors.Is check for ifacehigherorder.ErrUpdate"
		if true {
			return ErrNotFound
		}
		return ErrUpdate
	})
	if err != nil {
		println(err.Error())
	}
}

// =============================================================================
// Test 4: Direct (non-interface) call still works
// =============================================================================

func RunWithCallback(fn func() error) error { // want RunWithCallback:`\[call:0\]`
	return fn()
}

func TestDirectHigherOrder() {
	err := RunWithCallback(func() error { // want "missing errors.Is check for ifacehigherorder.ErrNotFound"
		return ErrNotFound
	})
	if err != nil {
		println(err.Error())
	}
}

// =============================================================================
// Test 5: Multiple implementations - intersection semantics
// =============================================================================

type AnotherRunner struct{}

func (a *AnotherRunner) RunInTx(fn func() error) error { // want RunInTx:`\[call:0\]`
	return fn()
}

// Both DBRunner and AnotherRunner have FunctionParamCallFlowFact for param 0,
// so the intersection should also have it.

// =============================================================================
// Test 6: Intersection failure - one impl does NOT call the function param
// =============================================================================

type MixedRunner interface {
	Run(fn func() error) error // want Run:`\[ifacehigherorder.ErrUpdate\]`
}

type CallingImpl struct{}

func (c *CallingImpl) Run(fn func() error) error { // want Run:`\[call:0\]`
	return fn()
}

type NonCallingImpl struct{}

func (n *NonCallingImpl) Run(fn func() error) error { // want Run:`\[ifacehigherorder.ErrUpdate\]`
	// Does NOT call fn - returns a fixed error instead
	return ErrUpdate
}

// MixedRunner should NOT have FunctionParamCallFlowFact because NonCallingImpl
// doesn't call the function param. So no warning about the lambda's errors,
// but there IS a warning about ErrUpdate from NonCallingImpl's InterfaceMethodFact.
func TestMixedRunner(r MixedRunner) {
	err := r.Run(func() error { // want "missing errors.Is check for ifacehigherorder.ErrUpdate"
		return ErrNotFound
	})
	if err != nil {
		println(err.Error())
	}
}
