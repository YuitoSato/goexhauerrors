package nested

import "errors"

// =============================================================================
// Sentinel Error Nested Cases
// =============================================================================

var ErrHoge = errors.New("hoge error") // want ErrHoge:`nested.ErrHoge`

func Nested3() error { // want Nested3:`\[nested.ErrHoge\]`
	return ErrHoge
}

func Nested2() error { // want Nested2:`\[nested.ErrHoge\]`
	err := Nested3()
	if err != nil {
		return err // SSA tracking: err holds ErrHoge from Nested3
	}
	return nil
}

func Nested1Bad() {
	err := Nested2() // want "missing errors.Is check for nested.ErrHoge"
	if err != nil {
		println(err.Error())
	}
}

func Nested1Good() {
	err := Nested2()
	if errors.Is(err, ErrHoge) {
		println("hoge error")
	}
}

// =============================================================================
// Multi-return Value Cases
// =============================================================================

func MultiReturn() (string, error) { // want MultiReturn:`\[nested.ErrHoge\]`
	return "", ErrHoge
}

func MultiReturnNested() (string, error) { // want MultiReturnNested:`\[nested.ErrHoge\]`
	val, err := MultiReturn()
	if err != nil {
		return "", err // SSA tracking: err from MultiReturn
	}
	return val, nil
}

func MultiReturnBadCaller() {
	_, err := MultiReturnNested() // want "missing errors.Is check for nested.ErrHoge"
	if err != nil {
		println(err.Error())
	}
}

// =============================================================================
// Custom Error Type Nested Cases
// =============================================================================

type ValidationError struct { // want ValidationError:`nested.ValidationError`
	Field string
}

func (e *ValidationError) Error() string {
	return "validation error: " + e.Field
}

func ValidateNested3() error { // want ValidateNested3:`\[nested.ValidationError\]`
	return &ValidationError{Field: "name"}
}

func ValidateNested2() error { // want ValidateNested2:`\[nested.ValidationError\]`
	err := ValidateNested3()
	if err != nil {
		return err // SSA tracking: err holds ValidationError from ValidateNested3
	}
	return nil
}

func ValidateNested1Bad() {
	err := ValidateNested2() // want "missing errors.Is check for nested.ValidationError"
	if err != nil {
		println(err.Error())
	}
}

func ValidateNested1Good() {
	err := ValidateNested2()
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		println("validation error on:", validationErr.Field)
	}
}

// =============================================================================
// Conditional Branch Cases (Phi nodes in SSA)
// =============================================================================

var ErrCondA = errors.New("condition A error") // want ErrCondA:`nested.ErrCondA`
var ErrCondB = errors.New("condition B error") // want ErrCondB:`nested.ErrCondB`

func GetCondAError() error { // want GetCondAError:`\[nested.ErrCondA\]`
	return ErrCondA
}

func GetCondBError() error { // want GetCondBError:`\[nested.ErrCondB\]`
	return ErrCondB
}

func ConditionalReturn(cond bool) error { // want ConditionalReturn:`\[nested.ErrCondA, nested.ErrCondB\]`
	var err error
	if cond {
		err = GetCondAError()
	} else {
		err = GetCondBError()
	}
	return err // SSA Phi node: err can be ErrCondA or ErrCondB
}

func ConditionalBadCaller() {
	err := ConditionalReturn(true) // want "missing errors.Is check for nested.ErrCondA" "missing errors.Is check for nested.ErrCondB"
	if err != nil {
		println(err.Error())
	}
}

func ConditionalGoodCaller() {
	err := ConditionalReturn(true)
	if errors.Is(err, ErrCondA) {
		println("condition A")
	} else if errors.Is(err, ErrCondB) {
		println("condition B")
	}
}

// =============================================================================
// Deep Nesting Cases
// =============================================================================

func DeepLevel4() error { // want DeepLevel4:`\[nested.ErrHoge\]`
	return ErrHoge
}

func DeepLevel3() error { // want DeepLevel3:`\[nested.ErrHoge\]`
	err := DeepLevel4()
	return err
}

func DeepLevel2() error { // want DeepLevel2:`\[nested.ErrHoge\]`
	err := DeepLevel3()
	if err != nil {
		return err
	}
	return nil
}

func DeepLevel1() error { // want DeepLevel1:`\[nested.ErrHoge\]`
	err := DeepLevel2()
	return err
}

func DeepBadCaller() {
	err := DeepLevel1() // want "missing errors.Is check for nested.ErrHoge"
	if err != nil {
		println(err.Error())
	}
}
