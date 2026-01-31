package usecase

import (
	"errors"
	"log"

	"crosspkgdi/domain"
)

// NOTE: usecase does NOT import infra — this is a real DI pattern.
// The linter should still detect that Repository.FindByID can return ErrNotFound
// and Repository.Save can return ValidationError.

type GetUseCase struct {
	repo domain.Repository
}

func NewGetUseCase(repo domain.Repository) *GetUseCase {
	return &GetUseCase{repo: repo}
}

// BadCaller does not check specific errors — should warn.
func (uc *GetUseCase) BadCaller(id string) string {
	val, err := uc.repo.FindByID(id) // want "missing errors.Is check for crosspkgdi/domain.ErrNotFound"
	if err != nil {
		log.Println(err)
		return ""
	}
	return val
}

// BadCallerSave does not check specific errors — should warn.
func (uc *GetUseCase) BadCallerSave(id string, value string) {
	err := uc.repo.Save(id, value) // want "missing errors.Is check for crosspkgdi/domain.ValidationError"
	if err != nil {
		log.Println(err)
	}
}

// GoodCaller checks all errors — no warning.
func (uc *GetUseCase) GoodCaller(id string) (string, error) {
	val, err := uc.repo.FindByID(id)
	if errors.Is(err, domain.ErrNotFound) {
		return "default", nil
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

// GoodCallerSave checks all errors — no warning.
func (uc *GetUseCase) GoodCallerSave(id string, value string) error {
	err := uc.repo.Save(id, value)
	var validErr *domain.ValidationError
	if errors.As(err, &validErr) {
		return err
	}
	return nil
}
