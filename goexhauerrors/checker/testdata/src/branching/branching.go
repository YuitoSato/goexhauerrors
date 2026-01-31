package branching

import "errors"

var ErrX = errors.New("x") // want ErrX:`branching.ErrX`
var ErrY = errors.New("y") // want ErrY:`branching.ErrY`

func TwoErrors(id string) error { // want TwoErrors:`\[branching.ErrX, branching.ErrY\]`
	if id == "x" {
		return ErrX
	}
	if id == "y" {
		return ErrY
	}
	return nil
}

// CheckInIfBranch checks one error in the if branch and the other in else.
// The OR merge strategy means both are considered checked.
func CheckInIfBranch() {
	err := TwoErrors("x")
	if err != nil {
		if errors.Is(err, ErrX) {
			println("x")
		}
	} else {
		if errors.Is(err, ErrY) {
			println("y")
		}
	}
}

// CheckAllInIfBody checks all errors inside an if body - no diagnostics.
func CheckAllInIfBody() {
	err := TwoErrors("x")
	if err != nil {
		if errors.Is(err, ErrX) {
			println("x")
		}
		if errors.Is(err, ErrY) {
			println("y")
		}
	}
}

// SwitchPartialCheck only checks one error in a switch case.
func SwitchPartialCheck() {
	err := TwoErrors("x") // want "missing errors.Is check for branching.ErrY"
	switch {
	case errors.Is(err, ErrX):
		println("x")
	}
}

// SwitchFullCheck checks all errors in switch cases - no diagnostics.
func SwitchFullCheck() {
	err := TwoErrors("x")
	switch {
	case errors.Is(err, ErrX):
		println("x")
	case errors.Is(err, ErrY):
		println("y")
	}
}

// NestedIf checks errors within nested if statements.
func NestedIf() {
	err := TwoErrors("x")
	if err != nil {
		if errors.Is(err, ErrX) {
			println("x")
		} else if errors.Is(err, ErrY) {
			println("y")
		}
	}
}
