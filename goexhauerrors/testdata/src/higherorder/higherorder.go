package higherorder

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("not found")   // want ErrNotFound:`higherorder.ErrNotFound`
var ErrUpdate = errors.New("update failed") // want ErrUpdate:`higherorder.ErrUpdate`

type Tx interface{}

type Conn struct{}

// RunInTx executes a function within a transaction
func (c *Conn) RunInTx(ctx context.Context, fn func(tx Tx) error) error { // want RunInTx:`\[call:1\]`
	return fn(nil)
}

// =============================================================================
// Test 1: Lambda passed directly to higher-order function - NOW detected
// =============================================================================

func TestHigherOrderLambda() {
	conn := &Conn{}
	ctx := context.Background()

	err := conn.RunInTx(ctx, func(tx Tx) error { // want "missing errors.Is check for higherorder.ErrNotFound"
		return ErrNotFound
	})

	if err != nil {
		println(err.Error())
	}
}

func TestHigherOrderLambdaGood() {
	conn := &Conn{}
	ctx := context.Background()

	err := conn.RunInTx(ctx, func(tx Tx) error {
		return ErrNotFound
	})

	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test 2: Lambda with multiple errors
// =============================================================================

func TestHigherOrderLambdaMultiple() {
	conn := &Conn{}
	ctx := context.Background()

	err := conn.RunInTx(ctx, func(tx Tx) error { // want "missing errors.Is check for higherorder.ErrNotFound" "missing errors.Is check for higherorder.ErrUpdate"
		if true {
			return ErrNotFound
		}
		return ErrUpdate
	})

	if err != nil {
		println(err.Error())
	}
}

func TestHigherOrderLambdaMultipleGood() {
	conn := &Conn{}
	ctx := context.Background()

	err := conn.RunInTx(ctx, func(tx Tx) error {
		if true {
			return ErrNotFound
		}
		return ErrUpdate
	})

	if errors.Is(err, ErrNotFound) {
		println("not found")
	} else if errors.Is(err, ErrUpdate) {
		println("update failed")
	}
}

// =============================================================================
// Test 3: Closure assigned to variable first (also works)
// =============================================================================

func TestAssignedClosure() {
	conn := &Conn{}
	ctx := context.Background()

	txFunc := func(tx Tx) error { // want txFunc:`\[higherorder.ErrNotFound\]`
		return ErrNotFound
	}

	err := conn.RunInTx(ctx, txFunc) // want "missing errors.Is check for higherorder.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func TestAssignedClosureGood() {
	conn := &Conn{}
	ctx := context.Background()

	txFunc := func(tx Tx) error { // want txFunc:`\[higherorder.ErrNotFound\]`
		return ErrNotFound
	}

	err := conn.RunInTx(ctx, txFunc)
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

// =============================================================================
// Test 4: Regular function (non-method)
// =============================================================================

func RunWithCallback(fn func() error) error { // want RunWithCallback:`\[call:0\]`
	return fn()
}

func TestRegularFunction() {
	err := RunWithCallback(func() error { // want "missing errors.Is check for higherorder.ErrNotFound"
		return ErrNotFound
	})
	if err != nil {
		println(err.Error())
	}
}

func TestRegularFunctionGood() {
	err := RunWithCallback(func() error {
		return ErrNotFound
	})
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}
