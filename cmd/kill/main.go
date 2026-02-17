// Command kill is a standalone entry point for the kill applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/kill"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(kill.Run(stdio, os.Args[1:]))
}
