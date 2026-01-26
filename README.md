# goexhauerrors

A Go static analysis tool that verifies all error types (sentinel errors and custom error types) returned by functions are exhaustively checked at call sites.

## Overview

In Go, `errors.Is` and `errors.As` are used to identify error types, but it's easy to forget to check all possible errors a function may return. This linter detects such omissions for both:
- **Sentinel errors**: `var Err* = errors.New("...")`
- **Custom error types**: Structs implementing the `error` interface

## Installation

```bash
go install github.com/yuito-sato/goexhauerrors@latest
```

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

## Detection Patterns

### 1. Sentinel Error Variables

Detects `var Err* = errors.New("...")` pattern:

```go
var ErrNotFound = errors.New("not found")
var ErrPermission = errors.New("permission denied")
```

### 2. Custom Error Types

Detects structs implementing the `error` interface:

```go
type ValidationError struct {
    Field string
}

func (e *ValidationError) Error() string {
    return "validation error: " + e.Field
}
```

### 3. Function and Method Return Analysis

Tracks which sentinel errors functions and methods may return:

```go
func GetItem(id string) (string, error) {
    if id == "" {
        return "", ErrNotFound
    }
    return "item", nil
}

func (s *Service) Delete(id string) error {
    if id == "" {
        return ErrNotFound
    }
    return nil
}
```

### 4. Wrapped Errors

Tracks errors wrapped with `fmt.Errorf` using `%w`:

```go
func Query() error {
    return fmt.Errorf("query failed: %w", ErrDatabase)
}
```

### 5. Error Propagation Through Variables (SSA-based)

Tracks errors assigned to variables and returned later using SSA dataflow analysis:

```go
func Nested3() error {
    return ErrNotFound
}

func Nested2() error {
    err := Nested3()  // SSA tracks: err holds ErrNotFound
    if err != nil {
        return err    // Detected: propagates ErrNotFound
    }
    return nil
}

func Caller() {
    err := Nested2() // Requires errors.Is check for ErrNotFound
}
```

Also works across packages:

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
    err := middle.PropagateViaVar() // Requires errors.Is check for ErrCrossPkg
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

### 6. Factory Functions

Tracks errors returned by factory functions through call chains:

```go
func NewValidationError(field string) error {
    return &ValidationError{Field: field}
}

func UseFactory() error {
    return NewValidationError("name") // Tracks ValidationError
}

func Caller() {
    err := UseFactory() // Requires errors.As check for ValidationError
}
```

### 7. Closures and Anonymous Functions

Tracks errors returned by closures assigned to variables:

```go
func UseClosure() {
    handler := func() error {
        return ErrHandler
    }
    err := handler() // Requires errors.Is check for ErrHandler
}
```

### 8. Variable Reassignment Scope

Properly handles variable reassignment with flow-sensitive analysis:

```go
func ReassignExample() {
    err := GetItem()  // ErrNotFound
    if errors.Is(err, ErrNotFound) {
        println("handled")
    }

    err = GetOther()  // ErrTimeout (new assignment, old sentinels cleared)
    // Only ErrTimeout check required here, not ErrNotFound
}
```

## Examples

### Code That Triggers Warnings

```go
func BadCaller() {
    _, err := GetItem("test") // missing errors.Is check for ErrNotFound
                              // missing errors.Is check for ErrPermission
    if err != nil {
        println(err.Error())
    }
}

func PartialCaller() {
    _, err := GetItem("test") // missing errors.Is check for ErrPermission
    if errors.Is(err, ErrNotFound) {
        println("not found")
    }
}
```

### Correct Code

```go
func GoodCaller() {
    _, err := GetItem("test")
    if errors.Is(err, ErrNotFound) {
        println("not found")
    } else if errors.Is(err, ErrPermission) {
        println("permission denied")
    }
}

func SwitchCaller() {
    _, err := GetItem("test")
    switch {
    case errors.Is(err, ErrNotFound):
        println("not found")
    case errors.Is(err, ErrPermission):
        println("permission denied")
    }
}
```

### Custom Error Types

```go
func GoodCallerWithCustomType() {
    err := Validate("")
    var validationErr *ValidationError
    if errors.As(err, &validationErr) {
        println("validation error on field:", validationErr.Field)
    }
}
```

### Error Propagation

No check required when propagating errors to the caller:

```go
func PropagatingCaller() error {
    _, err := GetItem("test")
    return err // OK - propagating error, no check needed
}
```

## Supported Checks

- `errors.Is(err, ErrXxx)` checks
- `errors.As(err, &target)` checks
- if-else chains
- switch statements
- Error propagation (return)
- Method calls on structs
- Factory function chains
- Closure variable calls
- Flow-sensitive variable reassignment
- **SSA-based variable tracking** (errors assigned to variables and returned)
- **Cross-package error propagation through variables**
- **Conditional branch merging (Phi nodes)**

## Limitations

The following patterns are **NOT** detected by this linter:

### 1. Errors from Function Parameters

```go
func WrapError(err error) error {
    return fmt.Errorf("wrapped: %w", err) // Cannot track what err is
}

func caller() {
    err := WrapError(ErrDatabase) // Linter silent - doesn't know ErrDatabase is wrapped
}
```

**Reason**: No data flow analysis for function parameters.

### 2. Errors through Interface Parameters

```go
type Reader interface {
    Read() error
}

func Use(r Reader) {
    err := r.Read() // Cannot determine which implementation returns which errors
}
```

**Reason**: When calling methods on interface parameters, the implementation is unknown at compile time.

**Note**: Concrete type method calls ARE detected:

```go
var m MyReader
err := m.Read() // This IS detected
```

### 3. Errors Stored in Structs or Maps

```go
type Container struct {
    Err error
}

func caller() {
    c := &Container{}
    _, c.Err = GetItem("test") // Field assignment not tracked
    return c.Err               // Unknown error type
}
```

**Reason**: Only direct variable assignments are tracked.

### 4. Type Assertions

```go
func process(i interface{}) error {
    return i.(error) // Unknown error type
}
```

**Reason**: Type narrowing not tracked.

### 5. External Packages Without Facts

```go
import "database/sql"

func Query() error {
    return sql.ErrNoRows // Not detected - no fact exported from stdlib
}
```

**Reason**: Cross-package analysis requires exported facts from the analyzed package.

### 6. Dynamic Error Creation

```go
func CreateError(msg string) error {
    return errors.New(msg) // Dynamic, not static sentinel
}
```

**Reason**: Only package-level `var Err* = errors.New(...)` patterns are detected.

## Summary Table

| Pattern | Detected | Notes |
|---------|----------|-------|
| Package-level sentinel vars | Yes | `var Err* = errors.New(...)` |
| Custom error types | Yes | Structs implementing `error` |
| Function/method returns | Yes | Direct sentinel returns |
| Wrapped errors (%w) | Yes | `fmt.Errorf` with `%w` |
| **Variable propagation** | **Yes** | **SSA-based: `err := F(); return err`** |
| **Cross-package via variable** | **Yes** | **SSA tracks through packages** |
| **Conditional branches (Phi)** | **Yes** | **`if cond { err = A() } else { err = B() }`** |
| **Multi-return extraction** | **Yes** | **`_, err := MultiReturn(); return err`** |
| Factory functions | Yes | Through call chain analysis |
| Closures | Yes | Assigned to variables |
| Variable reassignment | Yes | Flow-sensitive analysis |
| Cross-package (with facts) | Yes | Importing analyzed packages |
| Generic functions | Yes | Type parameters supported |
| Concrete type methods | Yes | `var m MyType; m.Method()` |
| Function parameters | No | No data flow tracking |
| Interface parameters | No | Implementation unknown |
| Struct/map storage | No | Field assignments |
| External packages (stdlib) | No | No facts available |
| Dynamic error creation | No | Only static patterns |

## License

MIT
