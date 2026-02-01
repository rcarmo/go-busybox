package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/mkdir"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(mkdir.Run(stdio, os.Args[1:]))
}
