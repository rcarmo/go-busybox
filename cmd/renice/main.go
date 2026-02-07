package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/renice"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(renice.Run(stdio, os.Args[1:]))
}
