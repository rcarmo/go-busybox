//go:build !js && !wasm && !wasip1

// Package who implements a minimal who command.
package who

import (
	"os"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) > 0 {
		return core.UsageError(stdio, "who", "invalid option -- '"+args[0]+"'")
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	tty := strings.TrimPrefix(os.Getenv("TTY"), "/dev/")
	if tty == "" {
		tty = "?"
	}
	stdio.Printf("%-8s %-12s ?\n", user, tty)
	return core.ExitSuccess
}
