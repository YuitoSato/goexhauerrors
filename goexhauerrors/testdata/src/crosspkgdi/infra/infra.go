package infra

import "crosspkgdi/domain"

// RepositoryImpl implements domain.Repository.
type RepositoryImpl struct{}

func (r *RepositoryImpl) FindByID(id string) (string, error) { // want FindByID:`\[crosspkgdi/domain.ErrNotFound\]`
	if id == "" {
		return "", domain.ErrNotFound
	}
	return "value", nil
}

func (r *RepositoryImpl) Save(id string, value string) error { // want Save:`\[crosspkgdi/domain.ValidationError\]`
	if id == "" {
		return &domain.ValidationError{Field: "id"}
	}
	return nil
}
