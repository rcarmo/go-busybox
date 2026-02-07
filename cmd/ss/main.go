package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/ss"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(ss.Run(stdio, os.Args[1:]))
}
