package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/nproc"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(nproc.Run(stdio, os.Args[1:]))
}
