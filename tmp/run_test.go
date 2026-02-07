//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"fmt"
	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"github.com/rcarmo/go-busybox/pkg/core"
	"strings"
)

func main() {
	out := &bytes.Buffer{}
	err := &bytes.Buffer{}
	stdio := &core.Stdio{In: strings.NewReader(""), Out: out, Err: err}
	code := awk.Run(stdio, []string{"BEGIN { print strftime(\"%s\", 0) }"})
	fmt.Printf("exit=%d\nOUT=%q\nERR=%q\n", code, out.String(), err.String())
}
