// Command ps is a standalone entry point for the ps applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/ps"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(ps.Run(stdio, os.Args[1:]))
}
