package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/ls"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(ls.Run(stdio, os.Args[1:]))
}
