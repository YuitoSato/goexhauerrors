package unexported

import "errors"

// Unexported sentinel errors - should NOT be detected
var errInternal = errors.New("internal error")
var errPrivate = errors.New("private error")

// Exported sentinel error - should be detected
var ErrPublic = errors.New("public error") // want ErrPublic:`unexported.ErrPublic`

// Unexported custom error type - should NOT be detected
type internalError struct {
	msg string
}

func (e *internalError) Error() string {
	return e.msg
}

// Exported custom error type - should be detected
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

// Function returning only unexported errors - no error facts expected
func DoPrivateWork() error {
	if true {
		return errInternal
	}
	return &internalError{msg: "internal"}
}

// Function returning mix of exported and unexported errors
// Only exported errors should be in the fact
func DoMixedWork() error { // want DoMixedWork:`\[unexported.ErrPublic\]`
	if true {
		return errPrivate // unexported, not tracked
	}
	return ErrPublic // exported, tracked
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

// Caller of function with only unexported errors - no warning needed
func PrivateCaller() {
	err := DoPrivateWork()
	if err != nil {
		println(err.Error())
	}
}
