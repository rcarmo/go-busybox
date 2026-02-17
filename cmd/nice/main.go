// Command nice is a standalone entry point for the nice applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/nice"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(nice.Run(stdio, os.Args[1:]))
}
