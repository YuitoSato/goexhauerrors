package sentinel

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found") // want ErrNotFound:"sentinel.ErrNotFound"

var ErrTimeout = fmt.Errorf("timeout") // want ErrTimeout:"sentinel.ErrTimeout"

var errPrivate = errors.New("private")

var NotAnError = "string"

var someErr = errors.New("some")
var ErrWrapped = fmt.Errorf("wrap: %w", someErr)

// Ensure errPrivate is used to avoid compile error.
var _ = errPrivate
