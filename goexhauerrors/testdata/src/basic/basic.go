package basic

import "errors"

var ErrNotFound = errors.New("not found")    // want ErrNotFound:`basic.ErrNotFound`
var ErrPermission = errors.New("permission") // want ErrPermission:`basic.ErrPermission`

func GetItem(id string) (string, error) { // want GetItem:`\[basic.ErrNotFound, basic.ErrPermission\]`
	if id == "" {
		return "", ErrNotFound
	}
	if id == "forbidden" {
		return "", ErrPermission
	}
	return "item", nil
}

func BadCaller() {
	_, err := GetItem("test") // want "missing errors.Is check for basic.ErrNotFound" "missing errors.Is check for basic.ErrPermission"
	if err != nil {
		println(err.Error())
	}
}

func PartialCaller() {
	_, err := GetItem("test") // want "missing errors.Is check for basic.ErrPermission"
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

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
