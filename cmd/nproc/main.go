// Command nproc is a standalone entry point for the nproc applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/nproc"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(nproc.Run(stdio, os.Args[1:]))
}
