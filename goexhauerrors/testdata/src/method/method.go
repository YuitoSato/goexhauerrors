package method

import "errors"

var ErrNotFound = errors.New("not found")   // want ErrNotFound:`method.ErrNotFound`
var ErrPermission = errors.New("permission") // want ErrPermission:`method.ErrPermission`

type Service struct{}

func (s *Service) GetItem(id string) (string, error) { // want GetItem:`\[method.ErrNotFound\]`
	if id == "" {
		return "", ErrNotFound
	}
	return "item", nil
}

func (s *Service) DeleteItem(id string) error { // want DeleteItem:`\[method.ErrNotFound, method.ErrPermission\]`
	if id == "" {
		return ErrNotFound
	}
	if id == "protected" {
		return ErrPermission
	}
	return nil
}

func BadCaller(s *Service) {
	_, err := s.GetItem("test") // want "missing errors.Is check for method.ErrNotFound"
	if err != nil {
		println(err.Error())
	}
}

func GoodCaller(s *Service) {
	_, err := s.GetItem("test")
	if errors.Is(err, ErrNotFound) {
		println("not found")
	}
}

func BadDeleteCaller(s *Service) {
	err := s.DeleteItem("test") // want "missing errors.Is check for method.ErrNotFound" "missing errors.Is check for method.ErrPermission"
	if err != nil {
		println(err.Error())
	}
}

func GoodDeleteCaller(s *Service) {
	err := s.DeleteItem("test")
	if errors.Is(err, ErrNotFound) {
		println("not found")
	} else if errors.Is(err, ErrPermission) {
		println("permission denied")
	}
}
