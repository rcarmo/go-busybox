// Command ls is a standalone entry point for the ls applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/ls"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(ls.Run(stdio, os.Args[1:]))
}
