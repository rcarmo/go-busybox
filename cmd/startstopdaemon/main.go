package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/startstopdaemon"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(startstopdaemon.Run(stdio, os.Args[1:]))
}
