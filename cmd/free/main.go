// Command free is a standalone entry point for the free applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/free"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(free.Run(stdio, os.Args[1:]))
}
