// Command head is a standalone entry point for the head applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/head"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(head.Run(stdio, os.Args[1:]))
}
