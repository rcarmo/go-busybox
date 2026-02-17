// Command cat is a standalone entry point for the cat applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/cat"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(cat.Run(stdio, os.Args[1:]))
}
