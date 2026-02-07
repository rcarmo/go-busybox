package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/tail"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(tail.Run(stdio, os.Args[1:]))
}
