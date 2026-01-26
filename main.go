package main

import (
	"github.com/yuito-sato/goexhauerrors/goexhauerrors"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(goexhauerrors.Analyzer)
}
