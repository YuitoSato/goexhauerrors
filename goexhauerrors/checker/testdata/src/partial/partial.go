package partial

import "errors"

var ErrFirst = errors.New("first")   // want ErrFirst:`partial.ErrFirst`
var ErrSecond = errors.New("second") // want ErrSecond:`partial.ErrSecond`
var ErrThird = errors.New("third")   // want ErrThird:`partial.ErrThird`

func ThreeErrors(id string) error { // want ThreeErrors:`\[partial.ErrFirst, partial.ErrSecond, partial.ErrThird\]`
	if id == "1" {
		return ErrFirst
	}
	if id == "2" {
		return ErrSecond
	}
	if id == "3" {
		return ErrThird
	}
	return nil
}

// CheckOnlyFirst checks ErrFirst but not ErrSecond or ErrThird.
func CheckOnlyFirst() {
	err := ThreeErrors("x") // want "missing errors.Is check for partial.ErrSecond" "missing errors.Is check for partial.ErrThird"
	if errors.Is(err, ErrFirst) {
		println("first")
	}
}

// CheckFirstAndSecond checks ErrFirst and ErrSecond but not ErrThird.
func CheckFirstAndSecond() {
	err := ThreeErrors("x") // want "missing errors.Is check for partial.ErrThird"
	if errors.Is(err, ErrFirst) {
		println("first")
	} else if errors.Is(err, ErrSecond) {
		println("second")
	}
}

// CheckAll checks all three errors - no diagnostics.
func CheckAll() {
	err := ThreeErrors("x")
	if errors.Is(err, ErrFirst) {
		println("first")
	} else if errors.Is(err, ErrSecond) {
		println("second")
	} else if errors.Is(err, ErrThird) {
		println("third")
	}
}

// CheckWithDirectComparePartial uses == for one and errors.Is for another.
func CheckWithDirectComparePartial() {
	err := ThreeErrors("x") // want "missing errors.Is check for partial.ErrThird"
	if err == ErrFirst {
		println("first")
	} else if errors.Is(err, ErrSecond) {
		println("second")
	}
}
