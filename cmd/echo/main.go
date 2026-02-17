// Command echo is a standalone entry point for the echo applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/echo"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(echo.Run(stdio, os.Args[1:]))
}
