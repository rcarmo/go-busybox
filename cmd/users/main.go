// Command users is a standalone entry point for the users applet.
package main

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/applets/users"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func main() {
	stdio := core.DefaultStdio()
	os.Exit(users.Run(stdio, os.Args[1:]))
}
