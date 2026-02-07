package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/rmdir"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(rmdir.Run(stdio, os.Args[1:]))
}
