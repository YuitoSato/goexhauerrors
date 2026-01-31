package ifacechecked

import "errors"

// =============================================================================
// Error definitions
// =============================================================================

var ErrC1 = errors.New("c1") // want ErrC1:`ifacechecked.ErrC1`
var ErrC2 = errors.New("c2") // want ErrC2:`ifacechecked.ErrC2`
var ErrC3 = errors.New("c3") // want ErrC3:`ifacechecked.ErrC3`

func getMultiErr() error { // want getMultiErr:`\[ifacechecked.ErrC1, ifacechecked.ErrC2, ifacechecked.ErrC3\]`
	if true {
		return ErrC1
	}
	if true {
		return ErrC2
	}
	return ErrC3
}

// =============================================================================
// Interface where ALL implementations check ErrC1 (but do NOT propagate)
// =============================================================================

type Checker interface {
	Check(err error) error // want Check:`\[param0:\[ifacechecked.ErrC1\]\]`
}

type CheckImpl1 struct{}

func (c *CheckImpl1) Check(err error) error { // want Check:`\[param0:\[ifacechecked.ErrC1\]\]`
	if errors.Is(err, ErrC1) {
		return nil
	}
	return errors.New("unhandled")
}

type CheckImpl2 struct{}

func (c *CheckImpl2) Check(err error) error { // want Check:`\[param0:\[ifacechecked.ErrC1\]\]`
	if errors.Is(err, ErrC1) {
		return nil
	}
	return errors.New("unhandled")
}

// Test: ErrC1 is checked by all impls inside Check, so only ErrC2 and ErrC3 should be warned
func TestCheckedViaInterfacePartial(c Checker) error { // want TestCheckedViaInterfacePartial:`\[call:0\]`
	err := getMultiErr() // want "missing errors.Is check for ifacechecked.ErrC2" "missing errors.Is check for ifacechecked.ErrC3"
	return c.Check(err)
}

// =============================================================================
// Interface where implementations check DIFFERENT errors (no intersection)
// =============================================================================

type DivergentChecker interface {
	DivCheck(err error) error
}

type DivImpl1 struct{}

func (d *DivImpl1) DivCheck(err error) error { // want DivCheck:`\[param0:\[ifacechecked.ErrC1\]\]`
	if errors.Is(err, ErrC1) {
		return nil
	}
	return errors.New("unhandled")
}

type DivImpl2 struct{}

func (d *DivImpl2) DivCheck(err error) error { // want DivCheck:`\[param0:\[ifacechecked.ErrC2\]\]`
	if errors.Is(err, ErrC2) {
		return nil
	}
	return errors.New("unhandled")
}

// Test: different errors checked in different impls → intersection is empty → all warned
func TestDivergentCheckedBad(d DivergentChecker) error { // want TestDivergentCheckedBad:`\[call:0\]`
	err := getMultiErr() // want "missing errors.Is check for ifacechecked.ErrC1" "missing errors.Is check for ifacechecked.ErrC2" "missing errors.Is check for ifacechecked.ErrC3"
	return d.DivCheck(err)
}

// =============================================================================
// Interface where ALL implementations check ErrC1 and ErrC2
// =============================================================================

type FullChecker interface {
	FullCheck(err error) error // want FullCheck:`\[param0:\[ifacechecked.ErrC1,ifacechecked.ErrC2\]\]`
}

type FullImpl1 struct{}

func (f *FullImpl1) FullCheck(err error) error { // want FullCheck:`\[param0:\[ifacechecked.ErrC1,ifacechecked.ErrC2\]\]`
	if errors.Is(err, ErrC1) {
		return nil
	}
	if errors.Is(err, ErrC2) {
		return nil
	}
	return errors.New("unhandled")
}

type FullImpl2 struct{}

func (f *FullImpl2) FullCheck(err error) error { // want FullCheck:`\[param0:\[ifacechecked.ErrC1,ifacechecked.ErrC2\]\]`
	if errors.Is(err, ErrC1) {
		return nil
	}
	if errors.Is(err, ErrC2) {
		return nil
	}
	return errors.New("unhandled")
}

// Test: ErrC1 and ErrC2 are checked by all impls → only ErrC3 warned
func TestFullCheckedViaInterface(fc FullChecker) error { // want TestFullCheckedViaInterface:`\[call:0\]`
	err := getMultiErr() // want "missing errors.Is check for ifacechecked.ErrC3"
	return fc.FullCheck(err)
}
