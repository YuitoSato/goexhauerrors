package unexported

import "errors"

// Unexported sentinel errors - now tracked locally (but not exported as facts)
var errInternal = errors.New("internal error")
var errPrivate = errors.New("private error")

// Exported sentinel error - detected and exported as fact
var ErrPublic = errors.New("public error") // want ErrPublic:`unexported.ErrPublic`

// Unexported custom error type - now tracked locally (but not exported as fact)
type internalError struct {
	msg string
}

func (e *internalError) Error() string {
	return e.msg
}

// Exported custom error type - detected and exported as fact
type PublicError struct { // want PublicError:`unexported.PublicError`
	Msg string
}

func (e *PublicError) Error() string {
	return e.Msg
}

// Function returning only exported errors
func DoPublicWork() error { // want DoPublicWork:`\[unexported.ErrPublic, unexported.PublicError\]`
	if true {
		return ErrPublic
	}
	return &PublicError{Msg: "public"}
}

// Function returning only unexported errors - now tracked locally
func DoPrivateWork() error { // want DoPrivateWork:`\[unexported.errInternal, unexported.internalError\]`
	if true {
		return errInternal
	}
	return &internalError{msg: "internal"}
}

// Function returning mix of exported and unexported errors
func DoMixedWork() error { // want DoMixedWork:`\[unexported.errPrivate, unexported.ErrPublic\]`
	if true {
		return errPrivate
	}
	return ErrPublic
}

// Caller that properly handles exported errors - no warning
func GoodCaller() {
	err := DoPublicWork()
	if errors.Is(err, ErrPublic) {
		println("public error")
	}
	var pubErr *PublicError
	if errors.As(err, &pubErr) {
		println("public error type")
	}
}

// Caller that doesn't handle exported errors - should warn
func BadCaller() {
	err := DoPublicWork() // want "missing errors.Is check for unexported.ErrPublic" "missing errors.Is check for unexported.PublicError"
	if err != nil {
		println(err.Error())
	}
}

// Caller of function with unexported errors - now warns
func PrivateCaller() {
	err := DoPrivateWork() // want "missing errors.Is check for unexported.errInternal" "missing errors.Is check for unexported.internalError"
	if err != nil {
		println(err.Error())
	}
}

// Caller that properly handles unexported errors
func GoodPrivateCaller() {
	err := DoPrivateWork()
	if errors.Is(err, errInternal) {
		println("internal error")
	}
	var intErr *internalError
	if errors.As(err, &intErr) {
		println("internal error type")
	}
}

// MixedPartialCaller checks only the exported error in a mixed function
func MixedPartialCaller() {
	err := DoMixedWork() // want "missing errors.Is check for unexported.errPrivate"
	if errors.Is(err, ErrPublic) {
		println("public")
	}
}

// MixedGoodCaller checks all errors (both exported and unexported) in a mixed function
func MixedGoodCaller() {
	err := DoMixedWork()
	if errors.Is(err, errPrivate) {
		println("private")
	}
	if errors.Is(err, ErrPublic) {
		println("public")
	}
}
