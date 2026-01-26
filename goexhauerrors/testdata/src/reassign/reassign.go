package reassign

import "errors"

var ErrNotFound = errors.New("not found") // want ErrNotFound:`reassign.ErrNotFound`
var ErrTimeout = errors.New("timeout")    // want ErrTimeout:`reassign.ErrTimeout`
var ErrInvalid = errors.New("invalid")    // want ErrInvalid:`reassign.ErrInvalid`

func GetItem() error { // want GetItem:`\[reassign.ErrNotFound\]`
	return ErrNotFound
}

func GetOther() error { // want GetOther:`\[reassign.ErrTimeout\]`
	return ErrTimeout
}

func GetThird() error { // want GetThird:`\[reassign.ErrInvalid\]`
	return ErrInvalid
}

// ReassignNoCheck: First call is checked, second is not
func ReassignNoCheck() {
	err := GetItem()
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}

	err = GetOther() // want "missing errors.Is check for reassign.ErrTimeout"
	if err != nil {
		println(err.Error())
	}
}

// ReassignAllChecked: Both calls are properly checked
func ReassignAllChecked() {
	err := GetItem()
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}

	err = GetOther()
	if errors.Is(err, ErrTimeout) {
		println("timeout")
	}
}

// ReassignFirstNotChecked: First call not checked, second is
func ReassignFirstNotChecked() {
	err := GetItem() // want "missing errors.Is check for reassign.ErrNotFound"
	if err != nil {
		println(err.Error())
	}

	err = GetOther()
	if errors.Is(err, ErrTimeout) {
		println("timeout")
	}
}

// ReassignNoneChecked: Neither call is checked
func ReassignNoneChecked() {
	err := GetItem() // want "missing errors.Is check for reassign.ErrNotFound"
	if err != nil {
		println(err.Error())
	}

	err = GetOther() // want "missing errors.Is check for reassign.ErrTimeout"
	if err != nil {
		println(err.Error())
	}
}

// MultipleReassigns: Multiple reassignments
func MultipleReassigns() {
	err := GetItem() // want "missing errors.Is check for reassign.ErrNotFound"
	_ = err

	err = GetOther() // want "missing errors.Is check for reassign.ErrTimeout"
	_ = err

	err = GetThird()
	if errors.Is(err, ErrInvalid) {
		println("invalid")
	}
}

// WrongCheckAfterReassign: Check for old sentinel after reassignment
func WrongCheckAfterReassign() {
	err := GetItem() // want "missing errors.Is check for reassign.ErrNotFound"
	_ = err

	err = GetOther() // want "missing errors.Is check for reassign.ErrTimeout"
	// This check is for ErrNotFound but err now holds result from GetOther
	if errors.Is(err, ErrNotFound) {
		println("not found - wrong!")
	}
}
