package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/tar"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(tar.Run(stdio, os.Args[1:]))
}
