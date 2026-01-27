package usecase

import (
	"context"

	cpkgerrors "crosspkgmethod/errors"
)

type UpdateUseCase struct{}

func NewUpdateUseCase() *UpdateUseCase {
	return &UpdateUseCase{}
}

// Execute executes the update operation.
func (uc *UpdateUseCase) Execute(ctx context.Context, tableID string) error { // want Execute:`\[crosspkgmethod/errors.ErrTableNotFound\]`
	if err := uc.validateTableExists(ctx, tableID); err != nil {
		return err
	}
	return nil
}

// validateTableExists validates that the table exists.
func (uc *UpdateUseCase) validateTableExists(ctx context.Context, tableID string) error { // want validateTableExists:`\[crosspkgmethod/errors.ErrTableNotFound\]`
	if tableID == "" {
		return cpkgerrors.ErrTableNotFound
	}
	return nil
}
