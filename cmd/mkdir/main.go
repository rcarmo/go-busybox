// Command mkdir is a standalone entry point for the mkdir applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/mkdir"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(mkdir.Run(stdio, os.Args[1:]))
}
