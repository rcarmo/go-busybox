package main

import (
	"os"

	"github.com/rcarmo/busybox-wasm/pkg/applets/echo"
	"github.com/rcarmo/busybox-wasm/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(echo.Run(stdio, os.Args[1:]))
}
