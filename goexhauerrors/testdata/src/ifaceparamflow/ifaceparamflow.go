package ifaceparamflow

import (
	"errors"
	"fmt"
)

// =============================================================================
// Error definitions
// =============================================================================

var ErrPF1 = errors.New("pf1") // want ErrPF1:`ifaceparamflow.ErrPF1`
var ErrPF2 = errors.New("pf2") // want ErrPF2:`ifaceparamflow.ErrPF2`

func getErr() error { // want getErr:`\[ifaceparamflow.ErrPF1\]`
	return ErrPF1
}

func getMultiErr() error { // want getMultiErr:`\[ifaceparamflow.ErrPF1, ifaceparamflow.ErrPF2\]`
	if true {
		return ErrPF1
	}
	return ErrPF2
}

// =============================================================================
// Interface where ALL implementations propagate error parameter
// =============================================================================

type Propagator interface {
	Propagate(err error) error // want Propagate:`\[0\]`
}

type PropImpl1 struct{}

func (p *PropImpl1) Propagate(err error) error { // want Propagate:`\[0\]`
	return err
}

type PropImpl2 struct{}

func (p *PropImpl2) Propagate(err error) error { // want Propagate:`\[0\]`
	return err
}

// Test: propagation through interface should be OK (all impls propagate)
func TestPropagateViaInterfaceGood(p Propagator) error { // want TestPropagateViaInterfaceGood:`\[ifaceparamflow.ErrPF1\]` TestPropagateViaInterfaceGood:`\[call:0\]`
	err := getErr()
	return p.Propagate(err) // OK - all impls propagate
}

// Test: propagation through interface with multiple errors should also propagate
func TestPropagateMultiGood(p Propagator) error { // want TestPropagateMultiGood:`\[ifaceparamflow.ErrPF1, ifaceparamflow.ErrPF2\]` TestPropagateMultiGood:`\[call:0\]`
	err := getMultiErr()
	return p.Propagate(err) // OK - all impls propagate
}

// =============================================================================
// Interface where NOT all implementations propagate
// =============================================================================

type PartialPropagator interface {
	MaybePropagate(err error) error
}

type PartialImpl1 struct{}

func (p *PartialImpl1) MaybePropagate(err error) error { // want MaybePropagate:`\[0\]`
	return err
}

type PartialImpl2 struct{}

func (p *PartialImpl2) MaybePropagate(err error) error {
	return errors.New("replaced")
}

// Test: should warn because not all impls propagate
func TestPartialPropagateBad(p PartialPropagator) error { // want TestPartialPropagateBad:`\[call:0\]`
	err := getErr() // want "missing errors.Is check for ifaceparamflow.ErrPF1"
	return p.MaybePropagate(err)
}

// =============================================================================
// Interface where all implementations wrap with %w
// =============================================================================

type Wrapper interface {
	Wrap(err error) error // want Wrap:`\[wrapped:0\]`
}

type WrapImpl1 struct{}

func (w *WrapImpl1) Wrap(err error) error { // want Wrap:`\[wrapped:0\]`
	return fmt.Errorf("impl1: %w", err)
}

type WrapImpl2 struct{}

func (w *WrapImpl2) Wrap(err error) error { // want Wrap:`\[wrapped:0\]`
	return fmt.Errorf("impl2: %w", err)
}

// Test: wrapping propagation through interface should be OK
func TestWrapViaInterfaceGood(w Wrapper) error { // want TestWrapViaInterfaceGood:`\[ifaceparamflow.ErrPF1\]` TestWrapViaInterfaceGood:`\[call:0\]`
	err := getErr()
	return w.Wrap(err) // OK - all impls wrap with %w
}

// =============================================================================
// Concrete type still detected normally alongside interface
// =============================================================================

func TestConcreteStillWorks() error { // want TestConcreteStillWorks:`\[ifaceparamflow.ErrPF1\]`
	p := &PropImpl1{}
	err := getErr()
	return p.Propagate(err) // OK - concrete propagation
}
