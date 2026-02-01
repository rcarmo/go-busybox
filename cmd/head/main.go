package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/head"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(head.Run(stdio, os.Args[1:]))
}
