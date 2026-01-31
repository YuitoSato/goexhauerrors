package checked

import "errors"

var ErrAlpha = errors.New("alpha") // want ErrAlpha:`checked.ErrAlpha`
var ErrBeta = errors.New("beta")   // want ErrBeta:`checked.ErrBeta`

func TwoErrors(id string) error { // want TwoErrors:`\[checked.ErrAlpha, checked.ErrBeta\]`
	if id == "a" {
		return ErrAlpha
	}
	if id == "b" {
		return ErrBeta
	}
	return nil
}

// GoodCallerErrorsIs properly checks all errors with errors.Is - no diagnostics.
func GoodCallerErrorsIs() {
	err := TwoErrors("x")
	if errors.Is(err, ErrAlpha) {
		println("alpha")
	} else if errors.Is(err, ErrBeta) {
		println("beta")
	}
}

// GoodCallerSwitch properly checks all errors in a switch - no diagnostics.
func GoodCallerSwitch() {
	err := TwoErrors("x")
	switch {
	case errors.Is(err, ErrAlpha):
		println("alpha")
	case errors.Is(err, ErrBeta):
		println("beta")
	}
}

// GoodCallerDirectCompare uses == for comparison - no diagnostics.
func GoodCallerDirectCompare() {
	err := TwoErrors("x")
	if err == ErrAlpha {
		println("alpha")
	} else if err == ErrBeta {
		println("beta")
	}
}

// GoodCallerDirectSwitchTag uses switch with error as tag - no diagnostics.
func GoodCallerDirectSwitchTag() {
	err := TwoErrors("x")
	switch err {
	case ErrAlpha:
		println("alpha")
	case ErrBeta:
		println("beta")
	}
}

// CustomError is a custom error type.
type CustomError struct { // want CustomError:`checked.CustomError`
	Msg string
}

func (e *CustomError) Error() string { return e.Msg }

func GetCustom() error { // want GetCustom:`\[checked.CustomError\]`
	return &CustomError{Msg: "custom"}
}

// GoodCallerErrorsAs properly checks using errors.As - no diagnostics.
func GoodCallerErrorsAs() {
	err := GetCustom()
	var ce *CustomError
	if errors.As(err, &ce) {
		println(ce.Msg)
	}
}

// GoodCallerTypeSwitch uses type switch - no diagnostics.
func GoodCallerTypeSwitch() {
	err := GetCustom()
	switch err.(type) {
	case *CustomError:
		println("custom")
	}
}
