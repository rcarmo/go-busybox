// Command tar is a standalone entry point for the tar applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/tar"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(tar.Run(stdio, os.Args[1:]))
}
