package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/gunzip"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(gunzip.Run(stdio, os.Args[1:]))
}
