// Command awk is a standalone entry point for the awk applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/awk"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(awk.Run(stdio, os.Args[1:]))
}
