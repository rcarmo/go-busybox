package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/wc"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(wc.Run(stdio, os.Args[1:]))
}
