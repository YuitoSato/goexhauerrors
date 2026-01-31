package caller

import (
	"errors"

	cpkgerrors "crosspkg/errors"
	"crosspkg/middle"
)

func BadCaller() {
	err := middle.PropagateViaVar() // want "missing errors.Is check for crosspkg/errors.ErrCrossPkg"
	if err != nil {
		println(err.Error())
	}
}

func BadCallerCustom() {
	err := middle.PropagateCustomViaVar() // want "missing errors.Is check for crosspkg/errors.CrossPkgError"
	if err != nil {
		println(err.Error())
	}
}

func BadCallerDirect() {
	err := middle.PropagateDirectReturn() // want "missing errors.Is check for crosspkg/errors.ErrCrossPkg"
	if err != nil {
		println(err.Error())
	}
}

func BadCallerBoth() {
	err := middle.PropagateBothViaVar(true) // want "missing errors.Is check for crosspkg/errors.CrossPkgError" "missing errors.Is check for crosspkg/errors.ErrCrossPkg"
	if err != nil {
		println(err.Error())
	}
}

func BadCallerHigherOrder() {
	err := middle.PropagateViaHigherOrderNamedFunc() // want "missing errors.Is check for crosspkg/errors.ErrCrossPkg"
	if err != nil {
		println(err.Error())
	}
}

func BadCallerDirectNamedFunc() {
	err := cpkgerrors.RunWithCallback(cpkgerrors.GetError) // want "missing errors.Is check for crosspkg/errors.ErrCrossPkg"
	if err != nil {
		println(err.Error())
	}
}

func GoodCallerDirectNamedFunc() {
	err := cpkgerrors.RunWithCallback(cpkgerrors.GetError)
	if errors.Is(err, cpkgerrors.ErrCrossPkg) {
		println("cross pkg error")
	}
}

func GoodCallerHigherOrder() {
	err := middle.PropagateViaHigherOrderNamedFunc()
	if errors.Is(err, cpkgerrors.ErrCrossPkg) {
		println("cross pkg error")
	}
}

func GoodCaller() {
	err := middle.PropagateViaVar()
	if errors.Is(err, cpkgerrors.ErrCrossPkg) {
		println("cross pkg error")
	}
}

func GoodCallerCustom() {
	err := middle.PropagateCustomViaVar()
	var cpErr *cpkgerrors.CrossPkgError
	if errors.As(err, &cpErr) {
		println("custom error:", cpErr.Code)
	}
}

func GoodCallerBoth() {
	err := middle.PropagateBothViaVar(true)
	if errors.Is(err, cpkgerrors.ErrCrossPkg) {
		println("sentinel error")
	}
	var cpErr *cpkgerrors.CrossPkgError
	if errors.As(err, &cpErr) {
		println("custom error:", cpErr.Code)
	}
}
