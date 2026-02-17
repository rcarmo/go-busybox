// Command nc is a standalone entry point for the nc applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/nc"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(nc.Run(stdio, os.Args[1:]))
}
