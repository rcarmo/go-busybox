// Command killall is a standalone entry point for the killall applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/killall"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(killall.Run(stdio, os.Args[1:]))
}
