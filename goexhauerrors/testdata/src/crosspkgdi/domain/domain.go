package domain

import "errors"

var ErrNotFound = errors.New("not found") // want ErrNotFound:`crosspkgdi/domain.ErrNotFound`

type ValidationError struct { // want ValidationError:`crosspkgdi/domain.ValidationError`
	Field string
}

func (e *ValidationError) Error() string {
	return "validation error: " + e.Field
}

// Validate is a regular exported function (not an interface method).
// Used to test that diagnostics are not duplicated when a function body
// contains both regular calls and interface method calls with global store misses.
func Validate(id string) error { // want Validate:`\[crosspkgdi/domain.ErrNotFound\]`
	if id == "" {
		return ErrNotFound
	}
	return nil
}

// Repository is an interface whose implementation is in a separate package (infra).
// The caller (usecase) imports only this package, not infra.
type Repository interface {
	FindByID(id string) (string, error)
	Save(id string, value string) error
}
