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

// MixedCaller calls both a regular function and an interface method without checking errors.
// This tests that each diagnostic is reported exactly once, even when deferred re-analysis
// re-walks the function body (the regular call's error must not be duplicated).
func (uc *GetUseCase) MixedCaller(id string) {
	err := domain.Validate(id) // want "missing errors.Is check for crosspkgdi/domain.ErrNotFound"
	log.Println(err)
	_, err = uc.repo.FindByID(id) // want "missing errors.Is check for crosspkgdi/domain.ErrNotFound"
	log.Println(err)
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

// BadCallerRunInTx does not check errors from higher-order call — should warn.
func (uc *GetUseCase) BadCallerRunInTx() {
	err := uc.repo.RunInTx(func() error { // want "missing errors.Is check for crosspkgdi/domain.ErrNotFound"
		return domain.ErrNotFound
	})
	if err != nil {
		log.Println(err)
	}
}

// GoodCallerRunInTx checks errors properly — no warning.
func (uc *GetUseCase) GoodCallerRunInTx() {
	err := uc.repo.RunInTx(func() error {
		return domain.ErrNotFound
	})
	if errors.Is(err, domain.ErrNotFound) {
		log.Println("not found")
	}
}
