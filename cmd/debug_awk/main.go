package main

import (
	"fmt"
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := &core.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
	args := []string{"BEGIN { printf \"%d\\n\", 3.0 }"}
	code := awk.Run(stdio, args)
	fmt.Fprintf(os.Stderr, "exit code: %d\n", code)
	os.Exit(code)
}
