package caller

import (
	"errors"

	cpkgerrors "crosspkgunexported/errors"
)

// --- Cross-package callers: unexported errors should NOT be detected ---

// BadCallerMixed calls a function returning both exported and unexported errors.
// Only exported errors should be warned - unexported errors from other packages
// cannot be checked with errors.Is/errors.As.
func BadCallerMixed() {
	err := cpkgerrors.DoWork(1) // want "missing errors.Is check for crosspkgunexported/errors.ErrPublic" "missing errors.Is check for crosspkgunexported/errors.PublicError"
	if err != nil {
		println(err.Error())
	}
}

// BadCallerPrivateOnly calls a function returning only unexported errors.
// No warnings should be emitted since none of the errors can be checked from outside.
func BadCallerPrivateOnly() {
	err := cpkgerrors.DoPrivateWork()
	if err != nil {
		println(err.Error())
	}
}

// GoodCallerMixed properly checks the exported errors - no warning.
func GoodCallerMixed() {
	err := cpkgerrors.DoWork(1)
	if errors.Is(err, cpkgerrors.ErrPublic) {
		println("public sentinel")
	}
	var pubErr *cpkgerrors.PublicError
	if errors.As(err, &pubErr) {
		println("public error type")
	}
}
