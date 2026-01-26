package customtype

import "errors"

// ValidationError is a custom error type
type ValidationError struct { // want ValidationError:`customtype.ValidationError`
	Field string
}

func (e *ValidationError) Error() string {
	return "validation error: " + e.Field
}

func Validate(field string) error { // want Validate:`\[customtype.ValidationError\]`
	if field == "" {
		return &ValidationError{Field: "name"}
	}
	return nil
}

func BadCaller() {
	err := Validate("") // want "missing errors.Is check for customtype.ValidationError"
	if err != nil {
		println(err.Error())
	}
}

func GoodCaller() {
	err := Validate("")
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		println("validation error on field:", validationErr.Field)
	}
}
