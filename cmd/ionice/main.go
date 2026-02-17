// Command ionice is a standalone entry point for the ionice applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/ionice"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(ionice.Run(stdio, os.Args[1:]))
}
