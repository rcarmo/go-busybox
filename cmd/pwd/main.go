package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/pwd"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(pwd.Run(stdio, os.Args[1:]))
}
