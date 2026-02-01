package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/tail"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(tail.Run(stdio, os.Args[1:]))
}
