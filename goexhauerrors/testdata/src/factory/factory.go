package factory

import "errors"

// ValidationError is a custom error type
type ValidationError struct { // want ValidationError:`factory.ValidationError`
	Field string
}

func (e *ValidationError) Error() string {
	return "validation error: " + e.Field
}

// NewValidationError creates a new ValidationError
func NewValidationError(field string) error { // want NewValidationError:`\[factory.ValidationError\]`
	return &ValidationError{Field: field}
}

// UseFactory calls NewValidationError and returns the result
func UseFactory() error { // want UseFactory:`\[factory.ValidationError\]`
	return NewValidationError("name")
}

// ChainedFactory demonstrates multi-level factory tracking
func ChainedFactory() error { // want ChainedFactory:`\[factory.ValidationError\]`
	return UseFactory()
}

func BadCaller() {
	err := UseFactory() // want "missing errors.Is check for factory.ValidationError"
	if err != nil {
		println(err.Error())
	}
}

func BadCallerDirect() {
	err := NewValidationError("field") // want "missing errors.Is check for factory.ValidationError"
	if err != nil {
		println(err.Error())
	}
}

func BadCallerChained() {
	err := ChainedFactory() // want "missing errors.Is check for factory.ValidationError"
	if err != nil {
		println(err.Error())
	}
}

func GoodCaller() {
	err := UseFactory()
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		println("validation error on field:", validationErr.Field)
	}
}
