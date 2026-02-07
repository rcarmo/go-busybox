//go:build !js && !wasm && !wasip1

// Package w implements a minimal w command.
package w

import (
	"os"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) > 0 {
		return core.UsageError(stdio, "w", "invalid option -- '"+args[0]+"'")
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	tty := strings.TrimPrefix(os.Getenv("TTY"), "/dev/")
	if tty == "" {
		tty = "?"
	}
	stdio.Println("USER     TTY      FROM             LOGIN@   IDLE   JCPU   PCPU WHAT")
	stdio.Printf("%-8s %-8s ?               ?       ?      ?      ?    -\n", user, tty)
	return core.ExitSuccess
}
