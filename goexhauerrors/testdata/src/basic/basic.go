package basic

import "errors"

var ErrNotFound = errors.New("not found")    // want ErrNotFound:`basic.ErrNotFound`
var ErrPermission = errors.New("permission") // want ErrPermission:`basic.ErrPermission`

func GetItem(id string) (string, error) { // want GetItem:`\[basic.ErrNotFound, basic.ErrPermission\]`
	if id == "" {
		return "", ErrNotFound
	}
	if id == "forbidden" {
		return "", ErrPermission
	}
	return "item", nil
}

func BadCaller() {
	_, err := GetItem("test") // want "missing errors.Is check for basic.ErrNotFound" "missing errors.Is check for basic.ErrPermission"
	if err != nil {
		println(err.Error())
	}
}

func PartialCaller() {
	_, err := GetItem("test") // want "missing errors.Is check for basic.ErrPermission"
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

func GoodCaller() {
	_, err := GetItem("test")
	if errors.Is(err, ErrNotFound) {
		println("not found")
	} else if errors.Is(err, ErrPermission) {
		println("permission denied")
	}
}

func SwitchCaller() {
	_, err := GetItem("test")
	switch {
	case errors.Is(err, ErrNotFound):
		println("not found")
	case errors.Is(err, ErrPermission):
		println("permission denied")
	}
}

// NotFoundError is a custom error type for testing errors.As
type NotFoundError struct { // want NotFoundError:`basic.NotFoundError`
	Resource string
}

func (e *NotFoundError) Error() string {
	return "not found: " + e.Resource
}

func GetResource(name string) error { // want GetResource:`\[basic.NotFoundError\]`
	if name == "" {
		return &NotFoundError{Resource: "unknown"}
	}
	return nil
}

// GetMixedErrors returns both sentinel and custom errors
func GetMixedErrors(id string) error { // want GetMixedErrors:`\[basic.ErrNotFound, basic.NotFoundError\]`
	if id == "" {
		return ErrNotFound
	}
	if id == "custom" {
		return &NotFoundError{Resource: id}
	}
	return nil
}

func BadCallerCustom() {
	err := GetResource("") // want "missing errors.Is check for basic.NotFoundError"
	if err != nil {
		println(err.Error())
	}
}

func GoodCallerCustom() {
	err := GetResource("")
	var notFoundErr *NotFoundError
	if errors.As(err, &notFoundErr) {
		println("not found:", notFoundErr.Resource)
	}
}

// =============================================================================
// Direct comparison tests (Issue 6)
// =============================================================================

// GoodCallerDirectCompare checks errors using direct == comparison
func GoodCallerDirectCompare() {
	_, err := GetItem("test")
	if err == ErrNotFound {
		println("not found")
	} else if err == ErrPermission {
		println("permission denied")
	}
}

// GoodCallerDirectCompareReverse checks errors using reversed == comparison
func GoodCallerDirectCompareReverse() {
	_, err := GetItem("test")
	if ErrNotFound == err {
		println("not found")
	} else if ErrPermission == err {
		println("permission denied")
	}
}

// GoodCallerDirectCompareNotEqual checks errors using != comparison
func GoodCallerDirectCompareNotEqual() {
	_, err := GetItem("test")
	if err != ErrNotFound {
		// not ErrNotFound
	}
	if err != ErrPermission {
		// not ErrPermission
	}
}

// PartialDirectCompare only checks one error with ==
func PartialDirectCompare() {
	_, err := GetItem("test") // want "missing errors.Is check for basic.ErrPermission"
	if err == ErrNotFound {
		println("not found")
	}
}

// NilCompareNoFalsePositive verifies that err == nil does not mark any error as checked
func NilCompareNoFalsePositive() {
	_, err := GetItem("test") // want "missing errors.Is check for basic.ErrNotFound" "missing errors.Is check for basic.ErrPermission"
	if err == nil {
		println("no error")
	}
}

// MixedCompareStyles uses both == and errors.Is
func MixedCompareStyles() {
	_, err := GetItem("test")
	if err == ErrNotFound {
		println("not found")
	} else if errors.Is(err, ErrPermission) {
		println("permission")
	}
}

// DirectCompareInSwitch uses switch with error as tag
func DirectCompareInSwitch() {
	_, err := GetItem("test")
	switch err {
	case ErrNotFound:
		println("not found")
	case ErrPermission:
		println("permission")
	}
}

// =============================================================================
// Type switch tests (Issue 3)
// =============================================================================

// GoodCallerTypeSwitch uses type switch to check custom error type
func GoodCallerTypeSwitch() {
	err := GetResource("")
	switch err.(type) {
	case *NotFoundError:
		println("not found")
	}
}

// GoodCallerTypeSwitchAssign uses type switch with assignment
func GoodCallerTypeSwitchAssign() {
	err := GetResource("")
	switch v := err.(type) {
	case *NotFoundError:
		println("not found:", v.Resource)
	}
}

// BadCallerTypeSwitch does not match the right type
func BadCallerTypeSwitch() {
	err := GetResource("") // want "missing errors.Is check for basic.NotFoundError"
	switch err.(type) {
	default:
		println(err)
	}
}

// PartialTypeSwitchMixed checks custom type via type switch but misses sentinel
func PartialTypeSwitchMixed() {
	err := GetMixedErrors("test") // want "missing errors.Is check for basic.ErrNotFound"
	switch err.(type) {
	case *NotFoundError:
		println("custom not found")
	}
}

// GoodMixedCheck uses errors.Is for sentinel and type switch for custom type
func GoodMixedCheck() {
	err := GetMixedErrors("test")
	if errors.Is(err, ErrNotFound) {
		println("sentinel not found")
	}
	switch err.(type) {
	case *NotFoundError:
		println("custom not found")
	}
}
