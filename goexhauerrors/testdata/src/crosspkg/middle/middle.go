package middle

import cpkgerrors "crosspkg/errors"

// PropagateViaVar propagates error through variable (SSA tracking needed)
func PropagateViaVar() error { // want PropagateViaVar:`\[crosspkg/errors.ErrCrossPkg\]`
	err := cpkgerrors.GetError()
	if err != nil {
		return err // SSA tracking: err holds ErrCrossPkg from cross-package call
	}
	return nil
}

// PropagateCustomViaVar propagates custom error through variable
func PropagateCustomViaVar() error { // want PropagateCustomViaVar:`\[crosspkg/errors.CrossPkgError\]`
	err := cpkgerrors.GetCustomError()
	if err != nil {
		return err
	}
	return nil
}

// PropagateDirectReturn directly returns the function call result (no variable)
// This should already work without SSA
func PropagateDirectReturn() error { // want PropagateDirectReturn:`\[crosspkg/errors.ErrCrossPkg\]`
	return cpkgerrors.GetError()
}

// PropagateViaHigherOrderNamedFunc wraps a cross-package higher-order function call
// with a named function argument. The errors from the named function should propagate
// through the FunctionParamCallFlowFact of RunWithCallback.
func PropagateViaHigherOrderNamedFunc() error { // want PropagateViaHigherOrderNamedFunc:`\[crosspkg/errors.ErrCrossPkg\]`
	return cpkgerrors.RunWithCallback(cpkgerrors.GetError)
}

// TransitiveHigherOrderWrapper is a transitive wrapper that passes fn to a
// cross-package higher-order function instead of calling it directly.
// This tests ImportObjectFact path for FunctionParamCallFlowFact.
func TransitiveHigherOrderWrapper(fn func() error) error { // want TransitiveHigherOrderWrapper:`\[call:0\]`
	return cpkgerrors.RunWithCallback(fn)
}

// PropagateBothViaVar propagates both sentinel and custom error
func PropagateBothViaVar(useCustom bool) error { // want PropagateBothViaVar:`\[crosspkg/errors.CrossPkgError, crosspkg/errors.ErrCrossPkg\]`
	var err error
	if useCustom {
		err = cpkgerrors.GetCustomError()
	} else {
		err = cpkgerrors.GetError()
	}
	return err
}
