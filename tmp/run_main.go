//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := &core.Stdio{In: strings.NewReader(""), Out: os.Stdout, Err: os.Stderr}
	code := awk.Run(stdio, []string{"BEGIN { print strftime(\"%s\", 0) }"})
	fmt.Fprintf(os.Stderr, "EXIT=%d\n", code)
}
