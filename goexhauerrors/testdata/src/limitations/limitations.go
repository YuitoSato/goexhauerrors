package limitations

import "errors"

var ErrTest = errors.New("test error") // want ErrTest:`limitations.ErrTest`

// =============================================================================
// Test 1: Errors from Function Parameters - IS detected
// =============================================================================

func WrapError(err error) error { // want WrapError:`\[0\]`
	return err
}

func TestFunctionParameter() {
	err := WrapError(ErrTest) // want "missing errors.Is check for limitations.ErrTest"
	if err != nil {
		println(err.Error())
	}
}

// =============================================================================
// Test 2: Interface with Concrete Type - IS detected
// =============================================================================

type ErrorReturner interface {
	GetError() error
}

type MyReturner struct{}

func (m *MyReturner) GetError() error { // want GetError:`\[limitations.ErrTest\]`
	return ErrTest
}

func TestConcreteType() {
	var m MyReturner
	err := m.GetError() // want "missing errors.Is check for limitations.ErrTest"
	if err != nil {
		println(err.Error())
	}
}

func TestInterfaceParameter(r ErrorReturner) {
	// Should NOT warn - interface implementation unknown
	err := r.GetError()
	if err != nil {
		println(err.Error())
	}
}

// =============================================================================
// Test 3: Struct Field Storage - NOT detected (correct)
// =============================================================================

type Container struct {
	Err error
}

func GetError() error { // want GetError:`\[limitations.ErrTest\]`
	return ErrTest
}

func TestStructStorage() {
	c := &Container{}
	c.Err = GetError()
	// Should NOT warn - field assignment not tracked
	if c.Err != nil {
		println(c.Err.Error())
	}
}

// =============================================================================
// Test 4: Generic Functions - IS detected
// =============================================================================

func GenericFunc[T any](val T) error { // want GenericFunc:`\[limitations.ErrTest\]`
	return ErrTest
}

func TestGeneric() {
	err := GenericFunc("test") // want "missing errors.Is check for limitations.ErrTest"
	if err != nil {
		println(err.Error())
	}
}

func TestGenericGood() {
	err := GenericFunc("test")
	if errors.Is(err, ErrTest) {
		println("test error")
	}
}
