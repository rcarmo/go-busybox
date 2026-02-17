// Command time is a standalone entry point for the time applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/time"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(time.Run(stdio, os.Args[1:]))
}
