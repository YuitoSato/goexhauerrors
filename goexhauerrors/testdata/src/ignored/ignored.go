package ignored

import "errors"

var ErrIgnored = errors.New("ignored error") // want ErrIgnored:`ignored.ErrIgnored`

type IgnoredError struct { // want IgnoredError:`ignored.IgnoredError`
	Code int
}

func (e *IgnoredError) Error() string { return "ignored" }

func GetIgnoredError() error { // want GetIgnoredError:`\[ignored.ErrIgnored\]`
	return ErrIgnored
}

func GetIgnoredCustomError() error { // want GetIgnoredCustomError:`\[ignored.IgnoredError\]`
	return &IgnoredError{Code: 1}
}
