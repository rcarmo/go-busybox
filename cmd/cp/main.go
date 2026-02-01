package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/cp"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(cp.Run(stdio, os.Args[1:]))
}
