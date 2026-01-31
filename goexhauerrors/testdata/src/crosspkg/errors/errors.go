package errors

import "errors"

var ErrCrossPkg = errors.New("cross package error") // want ErrCrossPkg:`crosspkg/errors.ErrCrossPkg`

type CrossPkgError struct { // want CrossPkgError:`crosspkg/errors.CrossPkgError`
	Code int
}

func (e *CrossPkgError) Error() string {
	return "cross pkg error"
}

func GetError() error { // want GetError:`\[crosspkg/errors.ErrCrossPkg\]`
	return ErrCrossPkg
}

func GetCustomError() error { // want GetCustomError:`\[crosspkg/errors.CrossPkgError\]`
	return &CrossPkgError{Code: 500}
}

// RunWithCallback is a higher-order function that calls the provided function.
func RunWithCallback(fn func() error) error { // want RunWithCallback:`\[call:0\]`
	return fn()
}
