// Command taskset is a standalone entry point for the taskset applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/taskset"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(taskset.Run(stdio, os.Args[1:]))
}
