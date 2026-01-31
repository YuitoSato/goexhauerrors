# goexhauerrors

A Go linter that ensures all error types returned by functions are exhaustively checked at call sites.

## What It Catches

```go
var ErrNotFound  = errors.New("not found")
var ErrPermission = errors.New("permission denied")

func GetItem(id string) (string, error) {
    if id == "" {
        return "", ErrNotFound
    }
    if !hasAccess(id) {
        return "", ErrPermission
    }
    return "item", nil
}
```

**Bad** - Only checks `err != nil`, missing specific error handling:

```go
_, err := GetItem("test")
if err != nil {       // goexhauerrors: missing errors.Is check for ErrNotFound, ErrPermission
    log.Fatal(err)
}
```

**Good** - All error types are exhaustively checked:

```go
_, err := GetItem("test")
if errors.Is(err, ErrNotFound) {
    println("not found")
} else if errors.Is(err, ErrPermission) {
    println("permission denied")
}
```

**Good** - Propagating the error to the caller (no check required):

```go
func Handler() error {
    _, err := GetItem("test")
    return fmt.Errorf("handler failed: %w", err)  // OK - wrapped and propagated
}
```

## Why?

Go's `errors.Is` and `errors.As` let callers inspect specific error types, but nothing enforces that callers actually check **all** possible errors. When a function adds a new error type, callers that only do `if err != nil` silently swallow it. This linter catches those gaps at compile time.

## Installation

```bash
go install github.com/YuitoSato/goexhauerrors@latest
```

## Usage

```bash
goexhauerrors ./...
```

### Ignoring Packages

Exclude specific packages (e.g., standard library, third-party) from error checking:

```bash
goexhauerrors -ignorePackages="gorm.io/gorm,database/sql" ./...
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

---

## Detected Patterns

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

Check with `errors.As` or type switch:

```go
err := Validate("")
var ve *ValidationError
if errors.As(err, &ve) {
    println("field:", ve.Field)
}
```

### Wrapped Errors (%w)

Errors wrapped with `fmt.Errorf` are tracked through the wrapping:

```go
func Query() error {
    return fmt.Errorf("query failed: %w", ErrDatabase)
}

func Caller() {
    err := Query()
    if errors.Is(err, ErrDatabase) {  // OK
        println("database error")
    }
}
```

Note: `%v` is NOT treated as propagation because the original error is lost:

```go
return fmt.Errorf("failed: %v", err)  // Warning: missing errors.Is check
```

### Variable Propagation (SSA-based)

Errors assigned to variables and returned later are tracked using SSA dataflow analysis:

```go
func Inner() error { return ErrDatabase }

func Outer() error {
    err := Inner()
    return err      // Detected: propagates ErrDatabase
}

func Caller() {
    err := Outer()  // Warning: missing errors.Is check for ErrDatabase
}
```

### Conditional Branches

Both branches of conditionals are tracked:

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

### Cross-Package Propagation

Error tracking works across package boundaries:

```go
// package errors
func GetError() error { return ErrCrossPkg }

// package middle
func Propagate() error {
    err := errors.GetError()
    return err  // SSA tracks cross-package propagation
}

// package caller
func BadCaller() {
    err := middle.Propagate()  // Warning: missing errors.Is check
}
```

### Factory Functions & Closures

```go
// Factory function
func NewValidationError(field string) error {
    return &ValidationError{Field: field}
}

// Closure
handler := func() error { return ErrHandler }
err := handler()  // Warning: missing errors.Is check for ErrHandler
```

### Interface Method Calls

All concrete implementations are analyzed, and the union of their errors is tracked:

```go
type Repository interface {
    Get(id string) error
}

type UserRepo struct{}
func (r *UserRepo) Get(id string) error { return ErrNotFound }

type CacheRepo struct{}
func (r *CacheRepo) Get(id string) error { return ErrCacheMiss }

func Use(repo Repository) {
    err := repo.Get("123")  // Warning: missing checks for ErrNotFound AND ErrCacheMiss
}
```

### Higher-Order Functions

Errors from functions passed as arguments are tracked:

```go
func RunInTx(fn func() error) error {
    return fn()
}

func Caller() {
    err := RunInTx(func() error {
        return ErrNotFound
    })
    // Warning: missing errors.Is check for ErrNotFound
}
```

### Function Parameter Tracking

Errors passed through function parameters are tracked, including chained wrappers:

```go
func WrapError(err error) error {
    return fmt.Errorf("wrapped: %w", err)
}

err := WrapError(ErrNotFound)  // Warning: missing errors.Is check for ErrNotFound
```

### Error Checking Inside Called Functions

When a function checks some errors internally, only the unchecked errors are reported:

```go
func MapError(err error) error {
    if errors.Is(err, ErrNotFound) {
        return errors.New("mapped: not found")
    }
    return nil
}

func Caller() error {
    err := GetItem()           // returns ErrNotFound, ErrPermission
    return MapError(err)       // Warning: missing errors.Is check for ErrPermission
    // ErrNotFound is already checked inside MapError
}
```

### Variable Reassignment

After reassignment, only the new error types are tracked:

```go
err := GetItem()   // ErrNotFound
if errors.Is(err, ErrNotFound) { /* handled */ }

err = GetOther()   // Reassigned: now only ErrTimeout is tracked
if errors.Is(err, ErrTimeout) { /* handled */ }
```

---

## Accepted Check Patterns

All of the following patterns are recognized as valid error checks:

| Pattern | Example |
|---------|---------|
| `errors.Is` | `errors.Is(err, ErrNotFound)` |
| `errors.As` | `errors.As(err, &validationErr)` |
| Direct comparison | `err == ErrNotFound` |
| Switch on error | `switch err { case ErrNotFound: }` |
| Type switch | `switch err.(type) { case *ValidationError: }` |
| Inside `defer` | `defer func() { if errors.Is(err, ...) }()` |
| Inside `select` | `select { case <-ch: errors.Is(err, ...) }` |
| Propagation (`return`) | `return err` or `return fmt.Errorf("...: %w", err)` |

---

## Limitations

| Pattern | Status |
|---------|--------|
| Unexported errors (cross-package) | Not tracked across packages (by design) |
| Struct/map field storage | Not tracked |
| Dynamic error creation (`errors.New(variable)`) | Not tracked |

### Ignoring Packages

External errors (e.g., `sql.ErrNoRows`, `gorm.ErrRecordNotFound`) should be converted to your domain errors at API boundaries. Use `-ignorePackages` to exclude them:

```bash
goexhauerrors -ignorePackages="gorm.io/gorm,database/sql,strconv" ./...
```

---

## Feature Summary

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
| | Function parameters | Yes |
| | Error checks inside called functions | Yes |
| | Interface method calls | Yes |
| | Higher-order functions (lambda) | Yes |
| Check Patterns | `errors.Is` / `errors.As` | Yes |
| | Direct comparison (`==` / `!=`) | Yes |
| | Type switch (`switch err.(type)`) | Yes |
| | Switch with error tag (`switch err`) | Yes |
| | Inside `defer` / `select` | Yes |
| Not Supported | Unexported errors (cross-package) | No |
| | Struct/map field storage | No |
| | Dynamic error creation | No |

## License

MIT
