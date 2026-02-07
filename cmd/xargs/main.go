package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/xargs"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(xargs.Run(stdio, os.Args[1:]))
}
