package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/find"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(find.Run(stdio, os.Args[1:]))
}
