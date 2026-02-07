package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/mv"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(mv.Run(stdio, os.Args[1:]))
}
