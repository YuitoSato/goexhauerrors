package funclit

import "errors"

var ErrNotFound = errors.New("not found")    // want ErrNotFound:`funclit.ErrNotFound`
var ErrPermission = errors.New("permission") // want ErrPermission:`funclit.ErrPermission`

func GetItem(id string) (string, error) { // want GetItem:`\[funclit.ErrNotFound, funclit.ErrPermission\]`
	if id == "" {
		return "", ErrNotFound
	}
	if id == "forbidden" {
		return "", ErrPermission
	}
	return "item", nil
}

// BadFuncLitCaller has an anonymous function that doesn't check errors
func BadFuncLitCaller() {
	fn := func() {
		_, err := GetItem("test") // want "missing errors.Is check for funclit.ErrNotFound" "missing errors.Is check for funclit.ErrPermission"
		if err != nil {
			println(err.Error())
		}
	}
	fn()
}

// GoodFuncLitCaller has an anonymous function that properly checks errors
func GoodFuncLitCaller() {
	fn := func() {
		_, err := GetItem("test")
		if errors.Is(err, ErrNotFound) {
			println("not found")
		} else if errors.Is(err, ErrPermission) {
			println("permission")
		}
	}
	fn()
}

// BadImmediatelyInvokedFuncLit calls a function literal immediately without checking errors
func BadImmediatelyInvokedFuncLit() {
	func() {
		_, err := GetItem("test") // want "missing errors.Is check for funclit.ErrNotFound" "missing errors.Is check for funclit.ErrPermission"
		if err != nil {
			println(err.Error())
		}
	}()
}

// FuncLitReturnsErrorPropagation tests FuncLit that returns error (canPropagate)
func FuncLitReturnsErrorPropagation() {
	fn := func() error {
		_, err := GetItem("test")
		if err != nil {
			return err // propagation — no warning needed
		}
		return nil
	}
	_ = fn()
}

// GoroutineFuncLit tests FuncLit inside a goroutine
func GoroutineFuncLit() {
	go func() {
		_, err := GetItem("test") // want "missing errors.Is check for funclit.ErrNotFound" "missing errors.Is check for funclit.ErrPermission"
		if err != nil {
			println(err.Error())
		}
	}()
}
