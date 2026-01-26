package main

import (
	"github.com/YuitoSato/goexhauerrors/goexhauerrors"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(goexhauerrors.Analyzer)
}
