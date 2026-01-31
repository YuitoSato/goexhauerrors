package errors

import "errors"

// Exported sentinel error
var ErrPublic = errors.New("public error") // want ErrPublic:`crosspkgunexported/errors.ErrPublic`

// Unexported sentinel error - tracked locally but not exported as ErrorFact
var errPrivate = errors.New("private error")

// Unexported custom error type
type internalError struct {
	msg string
}

func (e *internalError) Error() string {
	return e.msg
}

// Exported custom error type
type PublicError struct { // want PublicError:`crosspkgunexported/errors.PublicError`
	Msg string
}

func (e *PublicError) Error() string {
	return e.Msg
}

// Function returning both exported and unexported errors
func DoWork(flag int) error { // want DoWork:`\[crosspkgunexported/errors.ErrPublic, crosspkgunexported/errors.PublicError, crosspkgunexported/errors.errPrivate, crosspkgunexported/errors.internalError\]`
	switch flag {
	case 1:
		return ErrPublic
	case 2:
		return &PublicError{Msg: "pub"}
	case 3:
		return errPrivate
	default:
		return &internalError{msg: "internal"}
	}
}

// Function returning only unexported errors
func DoPrivateWork() error { // want DoPrivateWork:`\[crosspkgunexported/errors.errPrivate, crosspkgunexported/errors.internalError\]`
	if true {
		return errPrivate
	}
	return &internalError{msg: "internal"}
}

// --- Same-package callers: unexported errors SHOULD be detected ---

// SamePackageBadCallerMixed does not check any errors - should warn about all (exported + unexported)
func SamePackageBadCallerMixed() {
	err := DoWork(1) // want "missing errors.Is check for crosspkgunexported/errors.ErrPublic" "missing errors.Is check for crosspkgunexported/errors.PublicError" "missing errors.Is check for crosspkgunexported/errors.errPrivate" "missing errors.Is check for crosspkgunexported/errors.internalError"
	if err != nil {
		println(err.Error())
	}
}

// SamePackageBadCallerPrivateOnly does not check unexported errors - should warn
func SamePackageBadCallerPrivateOnly() {
	err := DoPrivateWork() // want "missing errors.Is check for crosspkgunexported/errors.errPrivate" "missing errors.Is check for crosspkgunexported/errors.internalError"
	if err != nil {
		println(err.Error())
	}
}

// SamePackageGoodCaller checks all errors including unexported - no warning
func SamePackageGoodCaller() {
	err := DoWork(1)
	if errors.Is(err, ErrPublic) {
		println("public sentinel")
	}
	var pubErr *PublicError
	if errors.As(err, &pubErr) {
		println("public error type")
	}
	if errors.Is(err, errPrivate) {
		println("private sentinel")
	}
	var intErr *internalError
	if errors.As(err, &intErr) {
		println("internal error type")
	}
}
