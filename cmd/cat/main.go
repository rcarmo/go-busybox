package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/cat"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(cat.Run(stdio, os.Args[1:]))
}
