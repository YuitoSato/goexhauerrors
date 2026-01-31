package impl

import (
	"errors"
	"fmt"

	"crosspkgiface/iface"
)

// =============================================================================
// Service implementations (DoWork returns different errors)
// =============================================================================

type ServiceImplA struct{}

func (s *ServiceImplA) DoWork() error { // want DoWork:`\[crosspkgiface/iface.ErrIfaceA\]`
	return iface.ErrIfaceA
}

type ServiceImplB struct{}

func (s *ServiceImplB) DoWork() error { // want DoWork:`\[crosspkgiface/iface.ErrIfaceB\]`
	return iface.ErrIfaceB
}

// =============================================================================
// Transformer implementations (all propagate error param)
// =============================================================================

type TransformImplA struct{}

func (t *TransformImplA) Transform(err error) error { // want Transform:`\[0\]`
	return err
}

type TransformImplB struct{}

func (t *TransformImplB) Transform(err error) error { // want Transform:`\[wrapped:0\]`
	return fmt.Errorf("transformed: %w", err)
}

// =============================================================================
// Mapper implementations (all check ErrIfaceA)
// =============================================================================

type MapImplA struct{}

func (m *MapImplA) Map(err error) error { // want Map:`\[param0:\[crosspkgiface/iface.ErrIfaceA\]\]`
	if errors.Is(err, iface.ErrIfaceA) {
		return nil
	}
	return errors.New("not handled")
}

type MapImplB struct{}

func (m *MapImplB) Map(err error) error { // want Map:`\[param0:\[crosspkgiface/iface.ErrIfaceA\]\]`
	if errors.Is(err, iface.ErrIfaceA) {
		return nil
	}
	return errors.New("not handled")
}
