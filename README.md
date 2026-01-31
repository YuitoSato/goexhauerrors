# goexhauerrors

A Go static analysis tool that verifies all error types (sentinel errors and custom error types) returned by functions are exhaustively checked at call sites.

## Overview

In Go, `errors.Is` and `errors.As` are used to identify error types, but it's easy to forget to check all possible errors a function may return. This linter detects such omissions for both:
- Sentinel errors: `var Err* = errors.New("...")`
- Custom error types: Structs implementing the `error` interface

## Installation

```bash
go install github.com/YuitoSato/goexhauerrors@latest
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

Use `errors.Is`, `errors.As`, direct comparison (`==`/`!=`), or type switch to check errors:

```go
// if-else chain with errors.Is
func GoodCaller() {
    _, err := GetItem("test")
    if errors.Is(err, ErrNotFound) {
        println("not found")
    } else if errors.Is(err, ErrPermission) {
        println("permission denied")
    }
}

// switch statement with errors.Is
func SwitchCaller() {
    _, err := GetItem("test")
    switch {
    case errors.Is(err, ErrNotFound):
        println("not found")
    case errors.Is(err, ErrPermission):
        println("permission denied")
    }
}

// direct comparison (== / !=)
func DirectCompareCaller() {
    _, err := GetItem("test")
    if err == ErrNotFound {
        println("not found")
    } else if err == ErrPermission {
        println("permission denied")
    }
}

// switch with error tag
func SwitchTagCaller() {
    _, err := GetItem("test")
    switch err {
    case ErrNotFound:
        println("not found")
    case ErrPermission:
        println("permission denied")
    }
}

// custom error types with errors.As
func CheckCustomType() {
    err := Validate("")
    var validationErr *ValidationError
    if errors.As(err, &validationErr) {
        println("validation error on field:", validationErr.Field)
    }
}

// type switch
func TypeSwitchCaller() {
    err := Validate("")
    switch err.(type) {
    case *ValidationError:
        println("validation error")
    }
}
```

### Checks Inside defer and select

Error checks inside `defer` statements and `select` case bodies are recognized:

```go
func DeferCaller() {
    _, err := GetItem("test")
    defer func() {
        if errors.Is(err, ErrNotFound) {
            println("not found")
        }
        if errors.Is(err, ErrPermission) {
            println("permission")
        }
    }()
}

func SelectCaller() {
    _, err := GetItem("test")
    ch := make(chan struct{})
    select {
    case <-ch:
        if errors.Is(err, ErrNotFound) {
            println("not found")
        }
        if errors.Is(err, ErrPermission) {
            println("permission")
        }
    }
}
```

### Function Literals

Error handling inside anonymous functions, immediately-invoked function literals, and goroutine closures is analyzed:

```go
func FuncLitCaller() {
    fn := func() {
        _, err := GetItem("test")
        // Warning: missing errors.Is check for ErrNotFound, ErrPermission
        if err != nil {
            println(err.Error())
        }
    }
    fn()
}
```

### Function Parameter Tracking

Errors passed through function parameters are tracked:

```go
func WrapError(err error) error {
    return fmt.Errorf("wrapped: %w", err)
}

func Caller() {
    err := WrapError(ErrNotFound)  // Warning: missing errors.Is check for ErrNotFound
    if err != nil {
        println(err.Error())
    }
}

func CallerGood() {
    err := WrapError(ErrNotFound)
    if errors.Is(err, ErrNotFound) {
        println("not found")  // OK
    }
}
```

Chained wrappers are also supported:

```go
func Wrapper1(err error) error { return Wrapper2(err) }
func Wrapper2(err error) error { return err }

func Test() {
    err := Wrapper1(ErrNotFound)  // Warning: ErrNotFound flows through both wrappers
}
```

### Interface Method Calls

Errors from interface method implementations are tracked by analyzing all concrete implementations:

```go
type Repository interface {
    Get(id string) error
}

type UserRepo struct{}

func (r *UserRepo) Get(id string) error {
    return ErrNotFound  // Tracked
}

func Use(repo Repository) {
    err := repo.Get("123")  // Warning: missing errors.Is check for ErrNotFound
}
```

### Higher-Order Functions (Lambda)

Errors from lambda functions passed to higher-order functions are detected:

```go
func RunInTx(fn func() error) error {
    return fn()
}

func Caller() {
    err := RunInTx(func() error {
        return ErrNotFound  // Tracked through higher-order function
    })
    // Warning: missing errors.Is check for ErrNotFound
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

Wrapping with `fmt.Errorf` using `%w` is also treated as propagation since the original error is still detectable via `errors.Is`:

```go
func Handler() error {
    _, err := GetItem("test")
    return fmt.Errorf("handler failed: %w", err)  // OK - error is wrapped but still detectable
}
```

Note: `fmt.Errorf` with `%v` is NOT treated as propagation because the original error is lost and cannot be detected via `errors.Is`:

```go
func Handler() error {
    _, err := GetItem("test")
    return fmt.Errorf("handler failed: %v", err)  // Warning: missing errors.Is check
}
```

### Error Checking Inside Called Functions

When a function receives an error parameter and checks it with `errors.Is`/`errors.As` internally, those checks are reflected to the caller. Only the **unchecked** errors are reported:

```go
var ErrNotFound = errors.New("not found")
var ErrPermission = errors.New("permission denied")
var ErrTimeout = errors.New("timeout")

func GetItem() error { /* returns all three errors */ }

// MapError checks 2 of 3 errors internally
func MapError(err error) error {
    if errors.Is(err, ErrNotFound) {
        return errors.New("mapped: not found")
    }
    if errors.Is(err, ErrPermission) {
        return errors.New("mapped: permission denied")
    }
    // ErrTimeout is NOT checked
    return nil
}

func Caller() error {
    err := GetItem()
    return MapError(err)  // Warning: missing errors.Is check for ErrTimeout
    // ErrNotFound and ErrPermission are already checked inside MapError
}
```

If the function checks **all** possible errors, no warning is reported:

```go
func MapErrorComplete(err error) error {
    if errors.Is(err, ErrNotFound) { return errors.New("mapped") }
    if errors.Is(err, ErrPermission) { return errors.New("mapped") }
    if errors.Is(err, ErrTimeout) { return errors.New("mapped") }
    return nil
}

func CallerGood() error {
    err := GetItem()
    return MapErrorComplete(err)  // OK - all errors checked inside MapErrorComplete
}
```

This also works with assignment (non-return) patterns:

```go
func CallerAssign() error {
    err := GetItem()
    result := MapError(err)  // Warning: missing errors.Is check for ErrTimeout
    return result
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

### Unexported Errors (Cross-Package)

Unexported errors are tracked within the same package but are not exported as facts for cross-package analysis:

```go
var errInternal = errors.New("internal")  // Tracked within the same package
var ErrPublic = errors.New("public")      // Tracked and exported for cross-package analysis
```

Callers in the same package will be warned about unchecked unexported errors. Callers in other packages will only see exported errors.

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

### Ignoring Packages

You can exclude specific packages from error checking using the `-ignorePackages` flag:

```bash
goexhauerrors -ignorePackages="gorm.io/gorm,database/sql" ./...
```

This is useful for excluding standard library or third-party package errors:

```go
import (
    "database/sql"
    "gorm.io/gorm"
)

func Query() error {
    return sql.ErrNoRows  // Ignored if "database/sql" is in ignorePackages
}

func FindUser() error {
    return gorm.ErrRecordNotFound  // Ignored if "gorm.io/gorm" is in ignorePackages
}
```

External errors should be converted to your domain errors at API boundaries.

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
| | Unexported errors (same package) | Yes |
| Tracking | Direct returns | Yes |
| | Wrapped errors (%w) | Yes |
| | Variable propagation (SSA-based) | Yes |
| | Cross-package propagation | Yes |
| | Conditional branches (Phi nodes) | Yes |
| | Factory functions | Yes |
| | Closures | Yes |
| | Function literals | Yes |
| | Variable reassignment | Yes |
| | Concrete type methods | Yes |
| | Function parameters | Yes |
| | Error checks inside called functions | Yes |
| | Interface method calls | Yes |
| | Higher-order functions (lambda) | Yes |
| Check Patterns | `errors.Is` / `errors.As` | Yes |
| | Direct comparison (`==` / `!=`) | Yes |
| | Type switch (`switch err.(type)`) | Yes |
| | Switch with error tag (`switch err`) | Yes |
| | Inside `defer` statements | Yes |
| | Inside `select` case bodies | Yes |
| Not Supported | Unexported errors (cross-package) | No (by design) |
| | Struct/map field storage | No |
| | Ignored packages | No (use -ignorePackages flag) |
| | Dynamic error creation | No |

## Usage

### Standalone

```bash
goexhauerrors ./...
```

### Ignoring Packages

Use the `-ignorePackages` flag to exclude specific packages from error checking:

```bash
# Ignore a single package
goexhauerrors -ignorePackages="gorm.io/gorm" ./...

# Ignore multiple packages (comma-separated)
goexhauerrors -ignorePackages="gorm.io/gorm,database/sql,strconv" ./...
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
