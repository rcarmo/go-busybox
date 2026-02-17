// Command wget is a standalone entry point for the wget applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/wget"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(wget.Run(stdio, os.Args[1:]))
}
