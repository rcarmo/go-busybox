//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"os"
)

func main() {
	state := &awk.awkState{vars: map[string]string{}, arrays: map[string]map[string]string{}, fs: " ", ofs: " "}
	e := &awk.expr{kind: awk.exprFunc, name: "strftime", args: []*awk.expr{{kind: awk.exprString, value: "%s"}, {kind: awk.exprNumber, num: 0, value: "0"}}}
	val, num := awk.EvalFuncForDebug(e, state)
	fmt.Printf("val=%q num=%v\n", val, num)
	os.Exit(0)
}
