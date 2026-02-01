package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/rmdir"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(rmdir.Run(stdio, os.Args[1:]))
}
