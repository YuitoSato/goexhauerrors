package iface

import "errors"

// =============================================================================
// Error definitions
// =============================================================================

var ErrIfaceA = errors.New("iface error a") // want ErrIfaceA:`crosspkgiface/iface.ErrIfaceA`
var ErrIfaceB = errors.New("iface error b") // want ErrIfaceB:`crosspkgiface/iface.ErrIfaceB`
var ErrIfaceC = errors.New("iface error c") // want ErrIfaceC:`crosspkgiface/iface.ErrIfaceC`

// =============================================================================
// Interface definitions (implementations are in separate package)
// =============================================================================

// Service is an interface whose implementations are in the impl package.
type Service interface {
	DoWork() error
}

// Transformer is an interface where all implementations propagate the error param.
type Transformer interface {
	Transform(err error) error
}

// Mapper is an interface where all implementations check ErrIfaceA.
type Mapper interface {
	Map(err error) error
}

// GetAllErrors is a helper that returns all errors for testing.
func GetAllErrors() error { // want GetAllErrors:`\[crosspkgiface/iface.ErrIfaceA, crosspkgiface/iface.ErrIfaceB, crosspkgiface/iface.ErrIfaceC\]`
	if true {
		return ErrIfaceA
	}
	if true {
		return ErrIfaceB
	}
	return ErrIfaceC
}
