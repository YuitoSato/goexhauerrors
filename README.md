# goexhauerrors

A Go static analysis tool that verifies all error types (sentinel errors and custom error types) returned by functions are exhaustively checked at call sites.

## Overview

In Go, `errors.Is` and `errors.As` are used to identify error types, but it's easy to forget to check all possible errors a function may return. This linter detects such omissions for both:
- Sentinel errors: `var Err* = errors.New("...")`
- Custom error types: Structs implementing the `error` interface

## Installation

```bash
go install github.com/yuito-sato/goexhauerrors@latest
```

## Quick Start

```go
var ErrNotFound = errors.New("not found")  // Detected as sentinel

func GetItem(id string) (string, error) {
    if id == "" {
        return "", ErrNotFound  // Tracked
    }
    return "item", nil
}

func main() {
    _, err := GetItem("")
    // Warning: missing errors.Is check for ErrNotFound
    if err != nil {
        log.Fatal(err)
    }
}
```

Run the linter:

```bash
goexhauerrors ./...
```

## What It Detects

### Sentinel Errors

Package-level variables matching `var Err* = errors.New("...")`:

```go
var ErrNotFound = errors.New("not found")
var ErrPermission = errors.New("permission denied")
```

### Custom Error Types

Structs implementing the `error` interface:

```go
type ValidationError struct {
    Field string
}

func (e *ValidationError) Error() string {
    return "validation error: " + e.Field
}
```

## How It Tracks Errors

### Direct Returns

Functions that directly return sentinel errors:

```go
func GetItem(id string) (string, error) {
    if id == "" {
        return "", ErrNotFound
    }
    return "item", nil
}
```

### Wrapped Errors (%w)

Errors wrapped with `fmt.Errorf` using `%w`:

```go
func Query() error {
    return fmt.Errorf("query failed: %w", ErrDatabase)
}
```

### Variable Propagation (SSA-based)

Errors assigned to variables and returned later are tracked using SSA dataflow analysis:

```go
func Inner() error {
    return ErrDatabase
}

func Outer() error {
    err := Inner()  // SSA tracks: err holds ErrDatabase
    return err      // Detected: propagates ErrDatabase
}

func Caller() {
    err := Outer()  // Warning: missing errors.Is check for ErrDatabase
}
```

Works across packages:

```go
// package errors
func GetError() error { return ErrCrossPkg }

// package middle
func PropagateViaVar() error {
    err := errors.GetError()
    return err  // SSA tracks cross-package propagation
}

// package caller
func BadCaller() {
    err := middle.PropagateViaVar()  // Warning: missing errors.Is check
}
```

Handles conditional branches (Phi nodes):

```go
func ConditionalReturn(cond bool) error {
    var err error
    if cond {
        err = GetErrorA()  // ErrA
    } else {
        err = GetErrorB()  // ErrB
    }
    return err  // Both ErrA and ErrB are tracked
}
```

### Factory Functions & Closures

Factory functions:

```go
func NewValidationError(field string) error {
    return &ValidationError{Field: field}
}

func UseFactory() error {
    return NewValidationError("name")  // Tracks ValidationError
}
```

Closures:

```go
func UseClosure() {
    handler := func() error {
        return ErrHandler
    }
    err := handler()  // Warning: missing errors.Is check for ErrHandler
}
```

## Call Site Analysis

### Required Checks

Use `errors.Is` or `errors.As` to check errors:

```go
// if-else chain
func GoodCaller() {
    _, err := GetItem("test")
    if errors.Is(err, ErrNotFound) {
        println("not found")
    } else if errors.Is(err, ErrPermission) {
        println("permission denied")
    }
}

// switch statement
func SwitchCaller() {
    _, err := GetItem("test")
    switch {
    case errors.Is(err, ErrNotFound):
        println("not found")
    case errors.Is(err, ErrPermission):
        println("permission denied")
    }
}

// custom error types
func CheckCustomType() {
    err := Validate("")
    var validationErr *ValidationError
    if errors.As(err, &validationErr) {
        println("validation error on field:", validationErr.Field)
    }
}
```

### No Check Required (Propagation)

When propagating errors to the caller, no check is required:

```go
func Handler() error {
    _, err := GetItem("test")
    return err  // OK - propagating error
}
```

### Variable Reassignment

After reassignment, only the new error types are tracked:

```go
func ReassignExample() {
    err := GetItem()  // ErrNotFound
    if errors.Is(err, ErrNotFound) {
        println("handled")
    }

    err = GetOther()  // Reassigned: now only ErrTimeout is tracked
    // Only ErrTimeout check required here
}
```

## Limitations

The following patterns are NOT detected:

### Errors from Function Parameters

```go
func WrapError(err error) error {
    return fmt.Errorf("wrapped: %w", err)  // Cannot track what err is
}
```

### Interface Method Calls

```go
type Reader interface {
    Read() error
}

func Use(r Reader) {
    err := r.Read()  // Implementation unknown at compile time
}
```

Note: Concrete type method calls ARE detected:

```go
var m MyReader
err := m.Read()  // This IS detected
```

### Struct/Map Field Storage

```go
type Container struct {
    Err error
}

func caller() {
    c := &Container{}
    _, c.Err = GetItem("test")  // Field assignment not tracked
}
```

### External Packages (stdlib)

```go
import "database/sql"

func Query() error {
    return sql.ErrNoRows  // No fact exported from stdlib
}
```

### Dynamic Error Creation

```go
func CreateError(msg string) error {
    return errors.New(msg)  // Dynamic, not static sentinel
}
```

## Summary

| Category | Pattern | Detected |
|----------|---------|----------|
| Definition | Sentinel vars (`var Err* = errors.New`) | Yes |
| | Custom error types | Yes |
| Tracking | Direct returns | Yes |
| | Wrapped errors (%w) | Yes |
| | Variable propagation (SSA-based) | Yes |
| | Cross-package propagation | Yes |
| | Conditional branches (Phi nodes) | Yes |
| | Factory functions | Yes |
| | Closures | Yes |
| | Variable reassignment | Yes |
| | Concrete type methods | Yes |
| Not Supported | Function parameters | No |
| | Interface method calls | No |
| | Struct/map field storage | No |
| | External packages (stdlib) | No |
| | Dynamic error creation | No |

## Usage

### Standalone

```bash
goexhauerrors ./...
```

### golangci-lint (Plugin)

`.golangci.yml`:

```yaml
linters-settings:
  custom:
    exhaustiveerrors:
      path: path/to/plugin.so
      description: Exhaustive error type checking
```

## License

MIT
