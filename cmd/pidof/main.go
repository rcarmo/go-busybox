package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/pidof"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(pidof.Run(stdio, os.Args[1:]))
}
