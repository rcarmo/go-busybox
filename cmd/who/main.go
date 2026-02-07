package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/who"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(who.Run(stdio, os.Args[1:]))
}
