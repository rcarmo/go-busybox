// Command wc is a standalone entry point for the wc applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/wc"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(wc.Run(stdio, os.Args[1:]))
}
