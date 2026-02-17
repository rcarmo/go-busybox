// Command pkill is a standalone entry point for the pkill applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/pkill"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(pkill.Run(stdio, os.Args[1:]))
}
