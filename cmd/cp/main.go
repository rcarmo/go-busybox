package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/cp"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(cp.Run(stdio, os.Args[1:]))
}
