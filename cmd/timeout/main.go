// Command timeout is a standalone entry point for the timeout applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/timeout"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(timeout.Run(stdio, os.Args[1:]))
}
