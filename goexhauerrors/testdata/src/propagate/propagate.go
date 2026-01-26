package propagate

import "errors"

var ErrNotFound = errors.New("not found") // want ErrNotFound:`propagate.ErrNotFound`

func GetItem(id string) (string, error) { // want GetItem:`\[propagate.ErrNotFound\]`
	if id == "" {
		return "", ErrNotFound
	}
	return "item", nil
}

// PropagatingCaller returns the error to caller - no check needed
// SSA analysis correctly detects that this function propagates ErrNotFound through variable
func PropagatingCaller() error { // want PropagatingCaller:`\[propagate.ErrNotFound\]`
	_, err := GetItem("test")
	return err // OK - propagating
}

// PropagatingWithWrap wraps and returns the error - no check needed
// SSA analysis correctly detects that this function propagates ErrNotFound through variable
func PropagatingWithWrap() error { // want PropagatingWithWrap:`\[propagate.ErrNotFound\]`
	_, err := GetItem("test")
	if err != nil {
		return err
	}
	return nil
}

// NonPropagatingCaller does not return error - must check
func NonPropagatingCaller() {
	_, err := GetItem("test") // want "missing errors.Is check for propagate.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}
