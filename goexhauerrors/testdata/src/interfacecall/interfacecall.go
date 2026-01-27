package interfacecall

import "errors"

// =============================================================================
// Error definitions
// =============================================================================

var ErrOne = errors.New("error one")   // want ErrOne:`interfacecall.ErrOne`
var ErrTwo = errors.New("error two")   // want ErrTwo:`interfacecall.ErrTwo`
var ErrThree = errors.New("error three") // want ErrThree:`interfacecall.ErrThree`

// CustomError is a custom error type
type CustomError struct { // want CustomError:`interfacecall.CustomError`
	Message string
}

func (e *CustomError) Error() string {
	return e.Message
}

// =============================================================================
// Interface with multiple implementations
// =============================================================================

type ErrorProducer interface {
	Produce() error // want Produce:`\[interfacecall.ErrOne, interfacecall.ErrTwo, interfacecall.CustomError\]`
}

// ImplA returns ErrOne
type ImplA struct{}

func (a *ImplA) Produce() error { // want Produce:`\[interfacecall.ErrOne\]`
	return ErrOne
}

// ImplB returns ErrTwo
type ImplB struct{}

func (b *ImplB) Produce() error { // want Produce:`\[interfacecall.ErrTwo\]`
	return ErrTwo
}

// ImplC returns CustomError
type ImplC struct{}

func (c *ImplC) Produce() error { // want Produce:`\[interfacecall.CustomError\]`
	return &CustomError{Message: "custom"}
}

// =============================================================================
// Test: Interface parameter with multiple implementations
// =============================================================================

func TestInterfaceCall(p ErrorProducer) {
	// Should warn for all errors from all implementations
	err := p.Produce() // want "missing errors.Is check for interfacecall.ErrOne" "missing errors.Is check for interfacecall.ErrTwo" "missing errors.Is check for interfacecall.CustomError"
	if err != nil {
		println(err.Error())
	}
}

func TestInterfaceCallPartialCheck(p ErrorProducer) {
	// Only check one error - should still warn for others
	err := p.Produce() // want "missing errors.Is check for interfacecall.ErrTwo" "missing errors.Is check for interfacecall.CustomError"
	if errors.Is(err, ErrOne) {
		println("error one")
	}
}

func TestInterfaceCallGood(p ErrorProducer) {
	// Check all errors - no warning
	err := p.Produce()
	if errors.Is(err, ErrOne) {
		println("error one")
	} else if errors.Is(err, ErrTwo) {
		println("error two")
	} else {
		var customErr *CustomError
		if errors.As(err, &customErr) {
			println(customErr.Message)
		}
	}
}

// =============================================================================
// Test: Interface variable
// =============================================================================

func TestInterfaceVariable() {
	var p ErrorProducer = &ImplA{}
	// Interface variable call - should warn for all implementations' errors
	err := p.Produce() // want "missing errors.Is check for interfacecall.ErrOne" "missing errors.Is check for interfacecall.ErrTwo" "missing errors.Is check for interfacecall.CustomError"
	if err != nil {
		println(err.Error())
	}
}

// =============================================================================
// Test: Concrete type is still detected normally
// =============================================================================

func TestConcreteType() {
	a := &ImplA{}
	// Concrete type call - should only warn for ErrOne
	err := a.Produce() // want "missing errors.Is check for interfacecall.ErrOne"
	if err != nil {
		println(err.Error())
	}
}

func TestConcreteTypeGood() {
	a := &ImplA{}
	err := a.Produce()
	if errors.Is(err, ErrOne) {
		println("error one")
	}
}

// =============================================================================
// Test: Interface with single implementation
// =============================================================================

type SingleProducer interface {
	Get() error // want Get:`\[interfacecall.ErrThree\]`
}

type OnlyImpl struct{}

func (o *OnlyImpl) Get() error { // want Get:`\[interfacecall.ErrThree\]`
	return ErrThree
}

func TestSingleImpl(s SingleProducer) {
	err := s.Get() // want "missing errors.Is check for interfacecall.ErrThree"
	if err != nil {
		println(err.Error())
	}
}

func TestSingleImplGood(s SingleProducer) {
	err := s.Get()
	if errors.Is(err, ErrThree) {
		println("error three")
	}
}
