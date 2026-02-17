// Command watch is a standalone entry point for the watch applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/watch"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(watch.Run(stdio, os.Args[1:]))
}
