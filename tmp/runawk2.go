//go:build ignore
// +build ignore

package main

import (
	"bytes"
	"fmt"
	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	outBuf := &bytes.Buffer{}
	stdio := &core.Stdio{In: nil, Out: outBuf, Err: outBuf}
	code := awk.Run(stdio, []string{"BEGIN { print strftime(\"%s\", 0) }"})
	fmt.Printf("exit=%d out=%q\n", code, outBuf.String())
}
