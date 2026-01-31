package higherorder

import (
	"context"
	"errors"
)

// =============================================================================
// Test: Named function passed to higher-order function
// This is the core case for issue #12 - named functions don't get their
// errors propagated through FunctionParamCallFlowFact in Phase 2
// =============================================================================

func namedTxFunc(tx Tx) error { // want namedTxFunc:`\[higherorder.ErrNotFound\]`
	return ErrNotFound
}

func wrapperWithNamedFunc() error { // want wrapperWithNamedFunc:`\[higherorder.ErrNotFound\]`
	conn := &Conn{}
	return conn.RunInTx(context.Background(), namedTxFunc)
}

// =============================================================================
// Test: Named function passed DIRECTLY to higher-order function at call site
// This tests Phase 3 checker's extractErrorsFromExpr handling of *types.Func
// =============================================================================

func TestDirectNamedFunc() {
	conn := &Conn{}
	ctx := context.Background()
	err := conn.RunInTx(ctx, namedTxFunc) // want "missing errors.Is check for higherorder.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func TestDirectNamedFuncGood() {
	conn := &Conn{}
	ctx := context.Background()
	err := conn.RunInTx(ctx, namedTxFunc)
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

func TestWrapperWithNamedFunc() {
	err := wrapperWithNamedFunc() // want "missing errors.Is check for higherorder.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func TestWrapperWithNamedFuncGood() {
	err := wrapperWithNamedFunc()
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test: Closure variable passed to higher-order function
// Note: This case already works via AST path (ast.Inspect traverses into
// the inline closure body). Included here for completeness.
// =============================================================================

func wrapperWithClosureVar() error { // want wrapperWithClosureVar:`\[higherorder.ErrNotFound\]`
	txFunc := func(tx Tx) error { // want txFunc:`\[higherorder.ErrNotFound\]`
		return ErrNotFound
	}
	conn := &Conn{}
	return conn.RunInTx(context.Background(), txFunc)
}

func TestWrapperWithClosureVar() {
	err := wrapperWithClosureVar() // want "missing errors.Is check for higherorder.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func TestWrapperWithClosureVarGood() {
	err := wrapperWithClosureVar()
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test: Named function with multiple errors passed to higher-order function
// =============================================================================

func namedTxFuncMultiple(tx Tx) error { // want namedTxFuncMultiple:`\[higherorder.ErrNotFound, higherorder.ErrUpdate\]`
	if true {
		return ErrNotFound
	}
	return ErrUpdate
}

func wrapperWithNamedFuncMultiple() error { // want wrapperWithNamedFuncMultiple:`\[higherorder.ErrNotFound, higherorder.ErrUpdate\]`
	conn := &Conn{}
	return conn.RunInTx(context.Background(), namedTxFuncMultiple)
}

func TestWrapperWithNamedFuncMultiple() {
	err := wrapperWithNamedFuncMultiple() // want "missing errors.Is check for higherorder.ErrNotFound" "missing errors.Is check for higherorder.ErrUpdate"
	if err != nil {
		println(err.Error())
	}
}

func TestWrapperWithNamedFuncMultipleGood() {
	err := wrapperWithNamedFuncMultiple()
	if errors.Is(err, ErrNotFound) {
		println("not found")
	} else if errors.Is(err, ErrUpdate) {
		println("update failed")
	}
}

// =============================================================================
// Test: Named function passed to non-method higher-order function
// =============================================================================

func namedCallbackFunc() error { // want namedCallbackFunc:`\[higherorder.ErrNotFound\]`
	return ErrNotFound
}

func wrapperWithNamedCallback() error { // want wrapperWithNamedCallback:`\[higherorder.ErrNotFound\]`
	return RunWithCallback(namedCallbackFunc)
}

func TestWrapperWithNamedCallback() {
	err := wrapperWithNamedCallback() // want "missing errors.Is check for higherorder.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func TestWrapperWithNamedCallbackGood() {
	err := wrapperWithNamedCallback()
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test: Two-level wrapper with named function
// =============================================================================

func outerWrapperNamedFunc() error { // want outerWrapperNamedFunc:`\[higherorder.ErrNotFound\]`
	return wrapperWithNamedFunc()
}

func TestOuterWrapperNamedFunc() {
	err := outerWrapperNamedFunc() // want "missing errors.Is check for higherorder.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func TestOuterWrapperNamedFuncGood() {
	err := outerWrapperNamedFunc()
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}
