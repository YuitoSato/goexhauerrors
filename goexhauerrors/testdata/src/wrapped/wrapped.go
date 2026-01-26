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

// DatabaseError is a custom error type for wrapped testing
type DatabaseError struct { // want DatabaseError:`wrapped.DatabaseError`
	Table string
}

func (e *DatabaseError) Error() string {
	return "database error on table: " + e.Table
}

func QueryCustom() error { // want QueryCustom:`\[wrapped.DatabaseError\]`
	return fmt.Errorf("query failed: %w", &DatabaseError{Table: "users"})
}

func BadCallerCustom() {
	err := QueryCustom() // want "missing errors.Is check for wrapped.DatabaseError"
	if err != nil {
		println(err.Error())
	}
}

func GoodCallerCustom() {
	err := QueryCustom()
	var dbErr *DatabaseError
	if errors.As(err, &dbErr) {
		println("database error on table:", dbErr.Table)
	}
}
