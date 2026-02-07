package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/sleep"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(sleep.Run(stdio, os.Args[1:]))
}
