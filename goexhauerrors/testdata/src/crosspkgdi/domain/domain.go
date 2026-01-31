package domain

import "errors"

var ErrNotFound = errors.New("not found") // want ErrNotFound:`crosspkgdi/domain.ErrNotFound`

type ValidationError struct { // want ValidationError:`crosspkgdi/domain.ValidationError`
	Field string
}

func (e *ValidationError) Error() string {
	return "validation error: " + e.Field
}

// Repository is an interface whose implementation is in a separate package (infra).
// The caller (usecase) imports only this package, not infra.
type Repository interface {
	FindByID(id string) (string, error)
	Save(id string, value string) error
}
