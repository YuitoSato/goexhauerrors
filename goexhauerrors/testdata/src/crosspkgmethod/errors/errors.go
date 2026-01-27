package errors

import "errors"

var ErrTableNotFound = errors.New("table not found") // want ErrTableNotFound:`crosspkgmethod/errors.ErrTableNotFound`
