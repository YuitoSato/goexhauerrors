package presentation

import (
	"context"
	"errors"
	"fmt"

	cpkgerrors "crosspkgmethod/errors"
	"crosspkgmethod/usecase"
)

// ErrInternal is an internal error.
var ErrInternal = errors.New("internal error") // want ErrInternal:`crosspkgmethod/presentation.ErrInternal`

// ErrNotFound is a not found error.
var ErrNotFound = errors.New("not found") // want ErrNotFound:`crosspkgmethod/presentation.ErrNotFound`

type Handler struct {
	updateUC *usecase.UpdateUseCase
}

func NewHandler(uc *usecase.UpdateUseCase) *Handler {
	return &Handler{updateUC: uc}
}

// Update handles the update request.
// ErrTableNotFound is NOT propagated because fmt.Errorf uses %v (not %w) for err.
func (h *Handler) Update(ctx context.Context, tableID string) error { // want Update:`\[crosspkgmethod/presentation.ErrNotFound, crosspkgmethod/presentation.ErrInternal\]`
	err := h.updateUC.Execute(ctx, tableID) // want "missing errors.Is check for crosspkgmethod/errors.ErrTableNotFound"
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			return fmt.Errorf("%w: %v", ErrNotFound, err)
		default:
			return fmt.Errorf("%w: %v", ErrInternal, err)
		}
	}
	return nil
}

// GoodUpdate properly checks for ErrTableNotFound.
func (h *Handler) GoodUpdate(ctx context.Context, tableID string) error { // want GoodUpdate:`\[crosspkgmethod/presentation.ErrNotFound, crosspkgmethod/presentation.ErrInternal\]`
	err := h.updateUC.Execute(ctx, tableID)
	if err != nil {
		switch {
		case errors.Is(err, cpkgerrors.ErrTableNotFound):
			return fmt.Errorf("%w: table not found", ErrNotFound)
		case errors.Is(err, ErrNotFound):
			return fmt.Errorf("%w: %v", ErrNotFound, err)
		default:
			return fmt.Errorf("%w: %v", ErrInternal, err)
		}
	}
	return nil
}
