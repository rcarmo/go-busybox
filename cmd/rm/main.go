// Command rm is a standalone entry point for the rm applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/rm"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(rm.Run(stdio, os.Args[1:]))
}
