package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/rm"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(rm.Run(stdio, os.Args[1:]))
}
