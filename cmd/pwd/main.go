package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/pwd"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(pwd.Run(stdio, os.Args[1:]))
}
