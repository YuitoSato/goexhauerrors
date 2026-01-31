package functiontype

import (
	"errors"
	"fmt"
)

var ErrHogeNotFound = errors.New("hoge not found") // want ErrHogeNotFound:`functiontype.ErrHogeNotFound`
var ErrHogeInvalid = errors.New("hoge invalid")    // want ErrHogeInvalid:`functiontype.ErrHogeInvalid`
var ErrFooNotFound = errors.New("foo not found")   // want ErrFooNotFound:`functiontype.ErrFooNotFound`
var ErrFooInvalid = errors.New("foo invalid")      // want ErrFooInvalid:`functiontype.ErrFooInvalid`

// =============================================================================
// Validate functions (each returns different errors)
// =============================================================================

func validateHoge() error { // want validateHoge:`\[functiontype.ErrHogeNotFound, functiontype.ErrHogeInvalid\]`
	if true {
		return ErrHogeNotFound
	}
	return ErrHogeInvalid
}

func validateFoo() error { // want validateFoo:`\[functiontype.ErrFooNotFound, functiontype.ErrFooInvalid\]`
	if true {
		return ErrFooNotFound
	}
	return ErrFooInvalid
}

// =============================================================================
// MapError variants
// =============================================================================

// MapErrorNG checks 3 of 4 errors — missing ErrFooInvalid.
func MapErrorNG(err error) error { // want MapErrorNG:`\[param0:\[functiontype.ErrHogeNotFound,functiontype.ErrHogeInvalid,functiontype.ErrFooNotFound\]\]`
	if errors.Is(err, ErrHogeNotFound) {
		return errors.New("mapped: hoge not found")
	}
	if errors.Is(err, ErrHogeInvalid) {
		return errors.New("mapped: hoge invalid")
	}
	if errors.Is(err, ErrFooNotFound) {
		return errors.New("mapped: foo not found")
	}
	return nil
}

// MapErrorOK checks all 4 errors.
func MapErrorOK(err error) error { // want MapErrorOK:`\[param0:\[functiontype.ErrHogeNotFound,functiontype.ErrHogeInvalid,functiontype.ErrFooNotFound,functiontype.ErrFooInvalid\]\]`
	if errors.Is(err, ErrHogeNotFound) {
		return errors.New("mapped: hoge not found")
	}
	if errors.Is(err, ErrHogeInvalid) {
		return errors.New("mapped: hoge invalid")
	}
	if errors.Is(err, ErrFooNotFound) {
		return errors.New("mapped: foo not found")
	}
	if errors.Is(err, ErrFooInvalid) {
		return errors.New("mapped: foo invalid")
	}
	return nil
}

func MapErrorOKWithReturnPropagation(err error) error { // want MapErrorOKWithReturnPropagation:`\[0\]`
	return err
}

func MapErrorOKWithFmtErrorfPropagation(err error) error { // want MapErrorOKWithFmtErrorfPropagation:`\[wrapped:0\]`
	return fmt.Errorf("mapped: %w", err)
}

// WrapError takes an error and propagates it directly (has ParameterFlowFact).
func WrapError(err error) error { // want WrapError:`\[0\]`
	return err
}

// =============================================================================
// Test 1: return MapErrorNG(err) with validateFoo — reports only ErrFooInvalid
// =============================================================================

func TestMapErrorNGWithFooBad() error {
	err := validateFoo() // want "missing errors.Is check for functiontype.ErrFooInvalid"
	return MapErrorNG(err)
}

// =============================================================================
// Test 2: return MapErrorOK(err) with validateFoo — no report (all checked)
// =============================================================================

func TestMapErrorOKWithFooGood() error {
	err := validateFoo()
	return MapErrorOK(err) // OK - all errors checked inside MapErrorOK
}

// =============================================================================
// Test 3: return MapErrorNG(err) with validateHoge — no report (all hoge errors checked)
// =============================================================================

func TestMapErrorNGWithHogeGood() error {
	err := validateHoge()
	return MapErrorNG(err) // OK - both ErrHogeNotFound and ErrHogeInvalid are checked
}

// =============================================================================
// Test 4: return MapErrorOK(err) with validateHoge — no report
// =============================================================================

func TestMapErrorOKWithHogeGood() error {
	err := validateHoge()
	return MapErrorOK(err) // OK - all errors checked
}

// =============================================================================
// Test 5: return WrapError(err) should NOT report (propagation via ParameterFlowFact)
// =============================================================================

func TestWrapHogeGood() error { // want TestWrapHogeGood:`\[functiontype.ErrHogeNotFound, functiontype.ErrHogeInvalid\]`
	err := validateHoge()
	return WrapError(err) // OK - propagated via ParameterFlowFact
}

func TestWrapFooGood() error { // want TestWrapFooGood:`\[functiontype.ErrFooNotFound, functiontype.ErrFooInvalid\]`
	err := validateFoo()
	return WrapError(err) // OK - propagated via ParameterFlowFact
}

// =============================================================================
// Test 6: return fmt.Errorf("...: %w", err) should NOT report (wrapping)
// =============================================================================

func TestFmtErrorfHogeGood() error {
	err := validateHoge()
	return fmt.Errorf("wrapped: %w", err) // OK - wrapping is propagation
}

func TestFmtErrorfFooGood() error {
	err := validateFoo()
	return fmt.Errorf("wrapped: %w", err) // OK - wrapping is propagation
}

// =============================================================================
// Test 7: return err directly should NOT report (direct propagation)
// =============================================================================

func TestDirectReturnHogeGood() error { // want TestDirectReturnHogeGood:`\[functiontype.ErrHogeNotFound, functiontype.ErrHogeInvalid\]`
	err := validateHoge()
	return err // OK - direct propagation
}

func TestDirectReturnFooGood() error { // want TestDirectReturnFooGood:`\[functiontype.ErrFooNotFound, functiontype.ErrFooInvalid\]`
	err := validateFoo()
	return err // OK - direct propagation
}

// =============================================================================
// Test 8: assignment with MapErrorNG (non-return) — reports missing ErrFooInvalid
// =============================================================================

func TestMapErrorNGAssignBad() error {
	err := validateFoo() // want "missing errors.Is check for functiontype.ErrFooInvalid"
	result := MapErrorNG(err)
	return result
}

// =============================================================================
// Test 9: assignment with MapErrorOK (non-return) — no report
// =============================================================================

func TestMapErrorOKAssignGood() error {
	err := validateFoo()
	result := MapErrorOK(err)
	return result
}

// =============================================================================
// Test 10: fmt.Errorf with %v does NOT propagate (unlike %w)
// =============================================================================

func TestFmtErrorfWithVHogeBad() error {
	err := validateHoge() // want "missing errors.Is check for functiontype.ErrHogeNotFound" "missing errors.Is check for functiontype.ErrHogeInvalid"
	return fmt.Errorf("not wrapped: %v", err)
}

func TestFmtErrorfWithVFooBad() error {
	err := validateFoo() // want "missing errors.Is check for functiontype.ErrFooNotFound" "missing errors.Is check for functiontype.ErrFooInvalid"
	return fmt.Errorf("not wrapped: %v", err)
}

// =============================================================================
// Test 11: return MapErrorOKWithReturnPropagation(err) — propagation via return
// =============================================================================

func TestReturnPropagationHogeGood() error { // want TestReturnPropagationHogeGood:`\[functiontype.ErrHogeNotFound, functiontype.ErrHogeInvalid\]`
	err := validateHoge()
	return MapErrorOKWithReturnPropagation(err) // OK - propagated (ParameterFlowFact)
}

func TestReturnPropagationFooGood() error { // want TestReturnPropagationFooGood:`\[functiontype.ErrFooNotFound, functiontype.ErrFooInvalid\]`
	err := validateFoo()
	return MapErrorOKWithReturnPropagation(err) // OK - propagated (ParameterFlowFact)
}

// =============================================================================
// Test 12: return MapErrorOKWithFmtErrorfPropagation(err) — propagation via %w
// =============================================================================

func TestFmtErrorfPropagationHogeGood() error { // want TestFmtErrorfPropagationHogeGood:`\[functiontype.ErrHogeNotFound, functiontype.ErrHogeInvalid\]`
	err := validateHoge()
	return MapErrorOKWithFmtErrorfPropagation(err) // OK - propagated (ParameterFlowFact with wrapped)
}

func TestFmtErrorfPropagationFooGood() error { // want TestFmtErrorfPropagationFooGood:`\[functiontype.ErrFooNotFound, functiontype.ErrFooInvalid\]`
	err := validateFoo()
	return MapErrorOKWithFmtErrorfPropagation(err) // OK - propagated (ParameterFlowFact with wrapped)
}
