package discard

import "errors"

var ErrNotFound = errors.New("not found")    // want ErrNotFound:`discard.ErrNotFound`
var ErrPermission = errors.New("permission") // want ErrPermission:`discard.ErrPermission`

// CustomError is a custom error type
type CustomError struct { // want CustomError:`discard.CustomError`
	Msg string
}

func (e *CustomError) Error() string { return e.Msg }

func GetItem(id string) (string, error) { // want GetItem:`\[discard.ErrNotFound, discard.ErrPermission\]`
	if id == "" {
		return "", ErrNotFound
	}
	if id == "forbidden" {
		return "", ErrPermission
	}
	return "item", nil
}

func GetError() error { // want GetError:`\[discard.ErrNotFound\]`
	return ErrNotFound
}

func GetCustomError() error { // want GetCustomError:`\[discard.CustomError\]`
	return &CustomError{Msg: "custom"}
}

// NoErrorFunc does not return any tracked errors
func NoErrorFunc() error {
	return nil
}

// --- ExprStmt: return value completely ignored ---

// DiscardReturnValue completely ignores the return value (multi-return)
func DiscardReturnValue() {
	GetItem("test") // want "error return value is discarded, missing check for discard.ErrNotFound" "error return value is discarded, missing check for discard.ErrPermission"
}

// DiscardSingleReturn ignores a single error return
func DiscardSingleReturn() {
	GetError() // want "error return value is discarded, missing check for discard.ErrNotFound"
}

// DiscardCustomError ignores a custom error type return
func DiscardCustomError() {
	GetCustomError() // want "error return value is discarded, missing check for discard.CustomError"
}

// DiscardNoTrackedError calls a function with no tracked errors - no warning
func DiscardNoTrackedError() {
	NoErrorFunc()
}

// --- Blank identifier ---

// BlankIdentifier assigns error to blank identifier (multi-return, all blank)
func BlankIdentifier() {
	_, _ = GetItem("test") // want "error assigned to blank identifier, missing check for discard.ErrNotFound" "error assigned to blank identifier, missing check for discard.ErrPermission"
}

// BlankIdentifierSingle assigns single error return to blank identifier
func BlankIdentifierSingle() {
	_ = GetError() // want "error assigned to blank identifier, missing check for discard.ErrNotFound"
}

// BlankIdentifierWithValue uses the non-error return but discards the error
func BlankIdentifierWithValue() {
	x, _ := GetItem("test") // want "error assigned to blank identifier, missing check for discard.ErrNotFound" "error assigned to blank identifier, missing check for discard.ErrPermission"
	println(x)
}

// --- Proper usage ---

// ProperUsage properly assigns and checks errors - no warning
func ProperUsage() {
	_, err := GetItem("test")
	if errors.Is(err, ErrNotFound) {
		println("not found")
	} else if errors.Is(err, ErrPermission) {
		println("permission")
	}
}
