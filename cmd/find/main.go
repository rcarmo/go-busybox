// Command find is a standalone entry point for the find applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/find"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(find.Run(stdio, os.Args[1:]))
}
