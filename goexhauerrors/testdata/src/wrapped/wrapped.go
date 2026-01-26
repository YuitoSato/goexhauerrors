package wrapped

import (
	"errors"
	"fmt"
)

var ErrDatabase = errors.New("database error") // want ErrDatabase:`wrapped.ErrDatabase`

func Query() error { // want Query:`\[wrapped.ErrDatabase\]`
	return fmt.Errorf("query failed: %w", ErrDatabase)
}

func BadCaller() {
	err := Query() // want "missing errors.Is check for wrapped.ErrDatabase"
	if err != nil {
		println(err.Error())
	}
}

func GoodCaller() {
	err := Query()
	if errors.Is(err, ErrDatabase) {
		println("database error")
	}
}
