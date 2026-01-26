package conditional

import "errors"

var ErrA = errors.New("error A") // want ErrA:`conditional.ErrA`
var ErrB = errors.New("error B") // want ErrB:`conditional.ErrB`

// ConditionalReturn returns ErrA only when flag is true
func ConditionalReturn(flag bool) error { // want ConditionalReturn:`\[conditional.ErrA\]`
	if flag {
		return ErrA
	}
	return nil
}

// MultipleConditional returns different errors based on conditions
func MultipleConditional(a, b bool) error { // want MultipleConditional:`\[conditional.ErrA, conditional.ErrB\]`
	if a {
		return ErrA
	}
	if b {
		return ErrB
	}
	return nil
}

// BadCaller does not check for ErrA
func BadCaller() {
	err := ConditionalReturn(true) // want "missing errors.Is check for conditional.ErrA"
	if err != nil {
		println(err.Error())
	}
}

// GoodCaller properly checks for ErrA
func GoodCaller() {
	err := ConditionalReturn(true)
	if errors.Is(err, ErrA) {
		println("error A")
	}
}

// BadMultipleCaller does not check all possible errors
func BadMultipleCaller() {
	err := MultipleConditional(true, false) // want "missing errors.Is check for conditional.ErrA" "missing errors.Is check for conditional.ErrB"
	if err != nil {
		println(err.Error())
	}
}

// PartialCaller only checks one of the possible errors
func PartialCaller() {
	err := MultipleConditional(true, false) // want "missing errors.Is check for conditional.ErrB"
	if errors.Is(err, ErrA) {
		println("error A")
	}
}

// GoodMultipleCaller checks all possible errors
func GoodMultipleCaller() {
	err := MultipleConditional(true, false)
	if errors.Is(err, ErrA) {
		println("error A")
	} else if errors.Is(err, ErrB) {
		println("error B")
	}
}
