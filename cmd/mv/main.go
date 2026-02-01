package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/mv"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(mv.Run(stdio, os.Args[1:]))
}
