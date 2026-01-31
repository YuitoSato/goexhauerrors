package switch_propagation

import (
	"errors"
	"fmt"
)

var ErrA = errors.New("a") // want ErrA:`switch_propagation.ErrA`
var ErrB = errors.New("b") // want ErrB:`switch_propagation.ErrB`
var ErrC = errors.New("c") // want ErrC:`switch_propagation.ErrC`

func ThreeErrors(id string) error { // want ThreeErrors:`\[switch_propagation.ErrA, switch_propagation.ErrB, switch_propagation.ErrC\]`
	switch id {
	case "a":
		return ErrA
	case "b":
		return ErrB
	case "c":
		return ErrC
	}
	return nil
}

// SwitchPropagationFalseNegative: ErrA and ErrB are checked with errors.Is
// and propagated via return err. ErrC falls into the catch-all branch where
// its identity is lost via fmt.Errorf with %v. Should report ErrC as unchecked.
func SwitchPropagationFalseNegative() (int, error) { // want SwitchPropagationFalseNegative:`\[switch_propagation.ErrA, switch_propagation.ErrB, switch_propagation.ErrC\]`
	err := ThreeErrors("x") // want "missing errors.Is check for switch_propagation.ErrC"
	switch {
	case errors.Is(err, ErrA) || errors.Is(err, ErrB):
		return 1, err
	case err != nil:
		return 0, fmt.Errorf("unexpected: %v", err)
	}
	return 0, nil
}

// SwitchPropagationAllChecked: All errors are checked with errors.Is and
// propagated. No diagnostic expected.
func SwitchPropagationAllChecked() (int, error) { // want SwitchPropagationAllChecked:`\[switch_propagation.ErrA, switch_propagation.ErrB, switch_propagation.ErrC\]`
	err := ThreeErrors("x")
	switch {
	case errors.Is(err, ErrA) || errors.Is(err, ErrB):
		return 1, err
	case errors.Is(err, ErrC):
		return 2, err
	}
	return 0, nil
}

// SwitchPropagationCatchAllReturn: All branches propagate the error.
// The catch-all case uses return err directly, so all errors are propagated.
// No diagnostic expected.
func SwitchPropagationCatchAllReturn() (int, error) { // want SwitchPropagationCatchAllReturn:`\[switch_propagation.ErrA, switch_propagation.ErrB, switch_propagation.ErrC\]`
	err := ThreeErrors("x")
	switch {
	case errors.Is(err, ErrA):
		return 1, err
	case err != nil:
		return 0, err
	}
	return 0, nil
}

// SwitchPropagationSingleCheck: Only ErrA is checked and propagated.
// ErrB and ErrC are unchecked.
func SwitchPropagationSingleCheck() (int, error) { // want SwitchPropagationSingleCheck:`\[switch_propagation.ErrA, switch_propagation.ErrB, switch_propagation.ErrC\]`
	err := ThreeErrors("x") // want "missing errors.Is check for switch_propagation.ErrB" "missing errors.Is check for switch_propagation.ErrC"
	switch {
	case errors.Is(err, ErrA):
		return 1, err
	case err != nil:
		return 0, fmt.Errorf("unexpected: %v", err)
	}
	return 0, nil
}

// SwitchTagPropagation: Only ErrA is properly checked and propagated.
// ErrB and ErrC should be reported.
func SwitchTagPropagation() (int, error) { // want SwitchTagPropagation:`\[switch_propagation.ErrA, switch_propagation.ErrB, switch_propagation.ErrC\]`
	err := ThreeErrors("x") // want "missing errors.Is check for switch_propagation.ErrB" "missing errors.Is check for switch_propagation.ErrC"
	switch {
	case errors.Is(err, ErrA):
		return 1, err
	}
	return 0, nil
}

// SwitchPropagationWithWrap: ErrA is checked and wrapped with %w (propagated).
// ErrB and ErrC fall into catch-all with %v (not propagated).
func SwitchPropagationWithWrap() (int, error) {
	err := ThreeErrors("x") // want "missing errors.Is check for switch_propagation.ErrB" "missing errors.Is check for switch_propagation.ErrC"
	switch {
	case errors.Is(err, ErrA):
		return 1, fmt.Errorf("wrapped: %w", err)
	case err != nil:
		return 0, fmt.Errorf("unexpected: %v", err)
	}
	return 0, nil
}

// SwitchTagWithPropagation: switch err { case ErrA: return err } syntax.
// Only ErrA is checked via direct comparison and propagated.
// ErrB and ErrC should be reported.
func SwitchTagWithPropagation() (int, error) { // want SwitchTagWithPropagation:`\[switch_propagation.ErrA, switch_propagation.ErrB, switch_propagation.ErrC\]`
	err := ThreeErrors("x") // want "missing errors.Is check for switch_propagation.ErrB" "missing errors.Is check for switch_propagation.ErrC"
	switch err {
	case ErrA:
		return 1, err
	default:
		return 0, fmt.Errorf("unexpected: %v", err)
	}
}

// SwitchDefaultPropagation: default clause propagates, so all errors are propagated.
// No diagnostic expected because the default clause is a catch-all and directly returns err.
func SwitchDefaultPropagation() (int, error) { // want SwitchDefaultPropagation:`\[switch_propagation.ErrA, switch_propagation.ErrB, switch_propagation.ErrC\]`
	err := ThreeErrors("x")
	switch {
	case errors.Is(err, ErrA):
		return 1, err
	default:
		return 0, err
	}
}
