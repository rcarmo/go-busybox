// Command logname is a standalone entry point for the logname applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/logname"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(logname.Run(stdio, os.Args[1:]))
}
