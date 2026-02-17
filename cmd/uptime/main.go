// Command uptime is a standalone entry point for the uptime applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/uptime"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(uptime.Run(stdio, os.Args[1:]))
}
