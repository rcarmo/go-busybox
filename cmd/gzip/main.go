package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/gzip"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(gzip.Run(stdio, os.Args[1:]))
}
