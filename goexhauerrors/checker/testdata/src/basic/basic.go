package basic

import "errors"

var ErrOne = errors.New("one") // want ErrOne:`basic.ErrOne`
var ErrTwo = errors.New("two") // want ErrTwo:`basic.ErrTwo`

func TwoErrors(id string) error { // want TwoErrors:`\[basic.ErrOne, basic.ErrTwo\]`
	if id == "one" {
		return ErrOne
	}
	if id == "two" {
		return ErrTwo
	}
	return nil
}

// UncheckedCaller does not check any errors - expects diagnostics for both.
func UncheckedCaller() {
	err := TwoErrors("x") // want "missing errors.Is check for basic.ErrOne" "missing errors.Is check for basic.ErrTwo"
	if err != nil {
		println(err.Error())
	}
}

// BlankIdentifier assigns to _ - no diagnostic expected.
func BlankIdentifier() {
	_ = TwoErrors("x")
}
