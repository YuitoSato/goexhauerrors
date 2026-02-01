package compositelit

import (
	"errors"
	"fmt"
)

// =============================================================================
// Pattern 1: Sentinel error with struct literal init (not errors.New)
// Exported custom error type with struct literal sentinels
// =============================================================================

type HTTPError struct { // want HTTPError:`compositelit.HTTPError`
	Code    int
	Message string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Code, e.Message)
}

var ErrNotFound = &HTTPError{Code: 404, Message: "not found"}
var ErrBadRequest = &HTTPError{Code: 400, Message: "bad request"}

// GetUser returns ErrNotFound (struct literal sentinel, NOT errors.New)
func GetUser(id string) (*string, error) { // want GetUser:`\[compositelit.HTTPError\]`
	if id == "" {
		return nil, ErrNotFound
	}
	name := "user"
	return &name, nil
}

// GetOrder returns ErrNotFound or ErrBadRequest
func GetOrder(id string) (*string, error) { // want GetOrder:`\[compositelit.HTTPError\]`
	if id == "" {
		return nil, ErrNotFound
	}
	if id == "bad" {
		return nil, ErrBadRequest
	}
	name := "order"
	return &name, nil
}

// BadCallerStructLiteral does not check for HTTPError type
func BadCallerStructLiteral() {
	_, err := GetUser("") // want "missing errors.Is check for compositelit.HTTPError"
	if err != nil {
		println(err.Error())
	}
}

// GoodCallerStructLiteral checks with errors.As
func GoodCallerStructLiteral() {
	_, err := GetUser("")
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		println("http error:", httpErr.Code)
	}
}

// =============================================================================
// Pattern 2: Exported variable of unexported custom error type (Echo pattern)
// The type is unexported, but the variable is exported
// =============================================================================

type httpErr struct {
	statusCode int
}

func (e *httpErr) Error() string {
	return fmt.Sprintf("status: %d", e.statusCode)
}

var ErrUnauthorized = &httpErr{statusCode: 401}
var ErrForbidden = &httpErr{statusCode: 403}

// Authenticate returns ErrUnauthorized or ErrForbidden
func Authenticate(token string) error { // want Authenticate:`\[compositelit.httpErr\]`
	if token == "" {
		return ErrUnauthorized
	}
	if token == "invalid" {
		return ErrForbidden
	}
	return nil
}

// BadCallerUnexportedType does not check at all
func BadCallerUnexportedType() {
	err := Authenticate("") // want "missing errors.Is check for compositelit.httpErr"
	if err != nil {
		println(err.Error())
	}
}

// GoodCallerUnexportedType checks via errors.As
func GoodCallerUnexportedType() {
	err := Authenticate("")
	var he *httpErr
	if errors.As(err, &he) {
		println("status:", he.statusCode)
	}
}

// =============================================================================
// Pattern 3: error-typed variable holding struct literal
// The variable is declared as `error` type, not the concrete type
// This was a false negative before the composite literal detection fix.
// =============================================================================

var ErrTypedAsError error = &HTTPError{Code: 500, Message: "internal"} // want ErrTypedAsError:`compositelit.ErrTypedAsError`

func GetInternalError() error { // want GetInternalError:`\[compositelit.ErrTypedAsError\]`
	return ErrTypedAsError
}

func BadCallerErrorTyped() {
	err := GetInternalError() // want "missing errors.Is check for compositelit.ErrTypedAsError"
	if err != nil {
		println(err.Error())
	}
}

func GoodCallerErrorTyped() {
	err := GetInternalError()
	if errors.Is(err, ErrTypedAsError) {
		println("typed as error")
	}
}

// =============================================================================
// Pattern 4: Custom constructor function (not errors.New directly)
// This is a known limitation - custom constructors are not recognized.
// =============================================================================

func newSentinelError(msg string) error {
	return errors.New(msg)
}

var ErrCustomInit = newSentinelError("custom initialized error")

func GetCustomError() error {
	return ErrCustomInit
}

// This is NOT detected - known limitation.
// ErrCustomInit is created by a custom constructor, not errors.New or composite literal.
func BadCallerCustomInit() {
	err := GetCustomError()
	if err != nil {
		println(err.Error())
	}
}

// =============================================================================
// Pattern 5: Multiple sentinels of the same custom type
// The linter reports at type level (HTTPError), not variable level.
// =============================================================================

func GetOrderBad(id string) error { // want GetOrderBad:`\[compositelit.HTTPError\]`
	if id == "" {
		return ErrNotFound
	}
	if id == "bad" {
		return ErrBadRequest
	}
	return nil
}

// Caller checks HTTPError type - this satisfies the linter
func CallerChecksSameType() {
	err := GetOrderBad("")
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		println("http error:", httpErr.Code)
	}
}

// =============================================================================
// Pattern 6: Sentinel + struct literal sentinel mix
// =============================================================================

var ErrSentinel = errors.New("sentinel") // want ErrSentinel:`compositelit.ErrSentinel`

func MixedReturns(flag int) error { // want MixedReturns:`\[compositelit.ErrSentinel, compositelit.HTTPError\]`
	switch flag {
	case 1:
		return ErrSentinel
	case 2:
		return ErrNotFound
	default:
		return nil
	}
}

func BadCallerMixed() {
	err := MixedReturns(1) // want "missing errors.Is check for compositelit.ErrSentinel" "missing errors.Is check for compositelit.HTTPError"
	if err != nil {
		println(err.Error())
	}
}

func PartialCallerMixed() {
	err := MixedReturns(1) // want "missing errors.Is check for compositelit.HTTPError"
	if errors.Is(err, ErrSentinel) {
		println("sentinel")
	}
}

// =============================================================================
// Pattern 7: Error wrapping method (like Echo's ErrBadRequest.Wrap(err))
// This is a known limitation - the linter cannot trace fmt.Errorf("%w", receiver)
// to discover the receiver's concrete error type.
// =============================================================================

func (e *HTTPError) Wrap(inner error) error {
	return fmt.Errorf("%w: %v", e, inner)
}

func BindJSON() error {
	return ErrBadRequest.Wrap(fmt.Errorf("invalid json"))
}

// Not detected - known limitation. The linter can't trace through
// fmt.Errorf's %w wrapping of a method receiver.
func BadCallerWrap() {
	err := BindJSON()
	if err != nil {
		println(err.Error())
	}
}

// =============================================================================
// Pattern 8: Type assertion pattern
// Direct type assertions (_, ok := err.(*Type)) are NOT recognized as checks.
// Only errors.Is, errors.As, direct == comparison, and type switch are supported.
// This is a known limitation.
// =============================================================================

func CallerTypeAssertionDirect() {
	_, err := GetUser("") // want "missing errors.Is check for compositelit.HTTPError"
	if _, ok := err.(*HTTPError); ok {
		println("http error")
	}
}
