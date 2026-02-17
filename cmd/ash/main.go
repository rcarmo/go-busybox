// Command ash is a standalone entry point for the ash applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/ash"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(ash.Run(stdio, os.Args[1:]))
}
