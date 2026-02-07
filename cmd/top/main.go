package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/top"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(top.Run(stdio, os.Args[1:]))
}
