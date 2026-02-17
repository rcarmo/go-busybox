//go:build !js && !wasm && !wasip1

// Package top implements a minimal top command.
package top

import (
	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the top command. This is a minimal stub that prints
// a static process listing header. No flags are supported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Println("PID USER COMMAND")
	stdio.Println("0 root [top]")
	return core.ExitSuccess
}
