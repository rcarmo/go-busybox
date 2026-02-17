// Command whoami is a standalone entry point for the whoami applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/whoami"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(whoami.Run(stdio, os.Args[1:]))
}
