package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/w"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(w.Run(stdio, os.Args[1:]))
}
