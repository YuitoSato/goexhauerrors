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

var ErrValidation = errors.New("validation failed") // want ErrValidation:`customtype.ErrValidation`

func ValidateSentinel(field string) error { // want ValidateSentinel:`\[customtype.ErrValidation\]`
	if field == "" {
		return ErrValidation
	}
	return nil
}

func BadCallerSentinel() {
	err := ValidateSentinel("") // want "missing errors.Is check for customtype.ErrValidation"
	if err != nil {
		println(err.Error())
	}
}

func GoodCallerSentinel() {
	err := ValidateSentinel("")
	if errors.Is(err, ErrValidation) {
		println("validation failed")
	}
}
