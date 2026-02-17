// Command pgrep is a standalone entry point for the pgrep applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/pgrep"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(pgrep.Run(stdio, os.Args[1:]))
}
