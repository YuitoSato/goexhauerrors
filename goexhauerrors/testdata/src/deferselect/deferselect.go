package deferselect

import "errors"

var ErrNotFound = errors.New("not found")    // want ErrNotFound:`deferselect.ErrNotFound`
var ErrPermission = errors.New("permission") // want ErrPermission:`deferselect.ErrPermission`

// CustomError is a custom error type
type CustomError struct { // want CustomError:`deferselect.CustomError`
	Msg string
}

func (e *CustomError) Error() string { return e.Msg }

func GetItem(id string) (string, error) { // want GetItem:`\[deferselect.ErrNotFound, deferselect.ErrPermission\]`
	if id == "" {
		return "", ErrNotFound
	}
	if id == "forbidden" {
		return "", ErrPermission
	}
	return "item", nil
}

func GetCustom() error { // want GetCustom:`\[deferselect.CustomError\]`
	return &CustomError{Msg: "err"}
}

// =============================================================================
// defer tests (Issue 7)
// =============================================================================

// DeferGoodCaller checks errors inside a defer
func DeferGoodCaller() {
	_, err := GetItem("test")
	defer func() {
		if errors.Is(err, ErrNotFound) {
			println("not found")
		}
		if errors.Is(err, ErrPermission) {
			println("permission")
		}
	}()
}

// DeferPartialCaller only checks one error inside defer
func DeferPartialCaller() {
	_, err := GetItem("test") // want "missing errors.Is check for deferselect.ErrPermission"
	defer func() {
		if errors.Is(err, ErrNotFound) {
			println("not found")
		}
	}()
}

// DeferWithErrorsAs checks custom error type inside defer using errors.As
func DeferWithErrorsAs() {
	err := GetCustom()
	defer func() {
		var ce *CustomError
		if errors.As(err, &ce) {
			println(ce.Msg)
		}
	}()
}

// DeferNoInterference ensures a plain defer does not affect error tracking
func DeferNoInterference() {
	_, err := GetItem("test")
	defer println("cleanup")
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
	if errors.Is(err, ErrPermission) {
		println("permission")
	}
}

// =============================================================================
// select tests (Issue 8)
// =============================================================================

// SelectGoodCaller checks errors inside select cases
func SelectGoodCaller() {
	_, err := GetItem("test")
	ch := make(chan struct{})
	select {
	case <-ch:
		if errors.Is(err, ErrNotFound) {
			println("not found")
		}
		if errors.Is(err, ErrPermission) {
			println("permission")
		}
	}
}

// SelectPartialCaller only checks one error inside select
func SelectPartialCaller() {
	_, err := GetItem("test") // want "missing errors.Is check for deferselect.ErrPermission"
	ch := make(chan struct{})
	select {
	case <-ch:
		if errors.Is(err, ErrNotFound) {
			println("not found")
		}
	}
}

// SelectMultipleCases checks different errors in different select cases
func SelectMultipleCases() {
	_, err := GetItem("test")
	ch1 := make(chan struct{})
	ch2 := make(chan struct{})
	select {
	case <-ch1:
		if errors.Is(err, ErrNotFound) {
			println("not found")
		}
	case <-ch2:
		if errors.Is(err, ErrPermission) {
			println("permission")
		}
	}
}

// SelectWithDefault checks errors in a select with default case
func SelectWithDefault() {
	_, err := GetItem("test")
	ch := make(chan struct{})
	select {
	case <-ch:
		if errors.Is(err, ErrNotFound) {
			println("not found")
		}
		if errors.Is(err, ErrPermission) {
			println("permission")
		}
	default:
		println("default")
	}
}
