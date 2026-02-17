// Command dig is a standalone entry point for the dig applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/dig"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(dig.Run(stdio, os.Args[1:]))
}
