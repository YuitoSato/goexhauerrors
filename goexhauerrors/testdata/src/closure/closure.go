package closure

import "errors"

var ErrHandler = errors.New("handler error")   // want ErrHandler:`closure.ErrHandler`
var ErrProcessor = errors.New("processor error") // want ErrProcessor:`closure.ErrProcessor`

func UseClosure() {
	handler := func() error { // want handler:`\[closure.ErrHandler\]`
		return ErrHandler
	}

	err := handler() // want "missing errors.Is check for closure.ErrHandler"
	if err != nil {
		println(err.Error())
	}
}

func UseClosureGood() {
	handler := func() error { // want handler:`\[closure.ErrHandler\]`
		return ErrHandler
	}

	err := handler()
	if errors.Is(err, ErrHandler) {
		println("handler error")
	}
}

func UseMultipleSentinels() {
	processor := func() error { // want processor:`\[closure.ErrHandler, closure.ErrProcessor\]`
		if true {
			return ErrHandler
		}
		return ErrProcessor
	}

	err := processor() // want "missing errors.Is check for closure.ErrHandler" "missing errors.Is check for closure.ErrProcessor"
	if err != nil {
		println(err.Error())
	}
}

func UseVarDeclaration() {
	var handler = func() error { // want handler:`\[closure.ErrHandler\]`
		return ErrHandler
	}

	err := handler() // want "missing errors.Is check for closure.ErrHandler"
	if err != nil {
		println(err.Error())
	}
}

// ClosureError is a custom error type for closure testing
type ClosureError struct { // want ClosureError:`closure.ClosureError`
	Message string
}

func (e *ClosureError) Error() string {
	return "closure error: " + e.Message
}

func UseClosureCustom() {
	handler := func() error { // want handler:`\[closure.ClosureError\]`
		return &ClosureError{Message: "from closure"}
	}

	err := handler() // want "missing errors.Is check for closure.ClosureError"
	if err != nil {
		println(err.Error())
	}
}

func UseClosureCustomGood() {
	handler := func() error { // want handler:`\[closure.ClosureError\]`
		return &ClosureError{Message: "from closure"}
	}

	err := handler()
	var closureErr *ClosureError
	if errors.As(err, &closureErr) {
		println("closure error:", closureErr.Message)
	}
}
