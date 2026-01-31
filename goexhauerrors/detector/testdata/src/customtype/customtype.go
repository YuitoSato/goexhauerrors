package customtype

// ValidationError is an exported custom error type with value receiver.
type ValidationError struct { // want ValidationError:"customtype.ValidationError"
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

// privateError is an unexported custom error type.
type privateError struct {
	msg string
}

func (e privateError) Error() string {
	return e.msg
}

// NotError does not implement the error interface.
type NotError struct {
	Value int
}

// PtrError implements the error interface with a pointer receiver.
type PtrError struct { // want PtrError:"customtype.PtrError"
	Code int
}

func (e *PtrError) Error() string {
	return "ptr error"
}
