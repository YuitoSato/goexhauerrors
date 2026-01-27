package paramflow

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found") // want ErrNotFound:`paramflow.ErrNotFound`
var ErrInvalid = errors.New("invalid")    // want ErrInvalid:`paramflow.ErrInvalid`

type CustomError struct { // want CustomError:`paramflow.CustomError`
	Message string
}

func (e *CustomError) Error() string { return e.Message }

// =============================================================================
// Test 1: Direct parameter return
// =============================================================================

func WrapSimple(err error) error { // want WrapSimple:`\[0\]`
	return err
}

func TestWrapSimpleBad() {
	err := WrapSimple(ErrNotFound) // want "missing errors.Is check for paramflow.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func TestWrapSimpleGood() {
	err := WrapSimple(ErrNotFound)
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test 2: fmt.Errorf wrapping
// =============================================================================

func WrapWithErrorf(err error) error { // want WrapWithErrorf:`\[wrapped:0\]`
	return fmt.Errorf("wrapped: %w", err)
}

func TestWrapWithErrorfBad() {
	err := WrapWithErrorf(ErrNotFound) // want "missing errors.Is check for paramflow.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func TestWrapWithErrorfGood() {
	err := WrapWithErrorf(ErrNotFound)
	if errors.Is(err, ErrNotFound) {
		println("found")
	}
}

// =============================================================================
// Test 3: Chained wrappers
// =============================================================================

func Wrapper1(err error) error { // want Wrapper1:`\[0\]`
	return Wrapper2(err)
}

func Wrapper2(err error) error { // want Wrapper2:`\[0\]`
	return err
}

func TestChainedBad() {
	err := Wrapper1(ErrNotFound) // want "missing errors.Is check for paramflow.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func TestChainedGood() {
	err := Wrapper1(ErrNotFound)
	if errors.Is(err, ErrNotFound) {
		println("found")
	}
}

// =============================================================================
// Test 4: Custom error type passed as parameter
// =============================================================================

func WrapCustom(err error) error { // want WrapCustom:`\[0\]`
	return err
}

func TestWrapCustomBad() {
	err := WrapCustom(&CustomError{Message: "test"}) // want "missing errors.Is check for paramflow.CustomError"
	if err != nil {
		println(err.Error())
	}
}

func TestWrapCustomGood() {
	err := WrapCustom(&CustomError{Message: "test"})
	var ce *CustomError
	if errors.As(err, &ce) {
		println(ce.Message)
	}
}
