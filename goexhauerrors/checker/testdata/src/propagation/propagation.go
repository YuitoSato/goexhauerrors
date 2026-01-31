package propagation

import (
	"errors"
	"fmt"
)

var ErrFoo = errors.New("foo") // want ErrFoo:`propagation.ErrFoo`
var ErrBar = errors.New("bar") // want ErrBar:`propagation.ErrBar`

func TwoErrors(id string) error { // want TwoErrors:`\[propagation.ErrFoo, propagation.ErrBar\]`
	if id == "foo" {
		return ErrFoo
	}
	if id == "bar" {
		return ErrBar
	}
	return nil
}

// DirectReturn propagates the error by returning it directly - no diagnostic.
func DirectReturn() error { // want DirectReturn:`\[propagation.ErrFoo, propagation.ErrBar\]`
	err := TwoErrors("x")
	return err
}

// ReturnInIf propagates the error in an if block - no diagnostic.
func ReturnInIf() error { // want ReturnInIf:`\[propagation.ErrFoo, propagation.ErrBar\]`
	err := TwoErrors("x")
	if err != nil {
		return err
	}
	return nil
}

// WrapWithFmtErrorf wraps the error using fmt.Errorf with %w - no diagnostic.
func WrapWithFmtErrorf() error {
	err := TwoErrors("x")
	if err != nil {
		return fmt.Errorf("wrapped: %w", err)
	}
	return nil
}

// NoReturn does not return error - must check.
func NoReturn() {
	err := TwoErrors("x") // want "missing errors.Is check for propagation.ErrFoo" "missing errors.Is check for propagation.ErrBar"
	if err != nil {
		println(err.Error())
	}
}
