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
func PropagatingCaller() error {
	_, err := GetItem("test")
	return err // OK - propagating
}

// PropagatingWithWrap wraps and returns the error - no check needed
func PropagatingWithWrap() error {
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
