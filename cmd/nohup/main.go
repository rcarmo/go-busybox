package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/nohup"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(nohup.Run(stdio, os.Args[1:]))
}
