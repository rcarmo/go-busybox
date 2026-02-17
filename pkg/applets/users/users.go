//go:build !js && !wasm && !wasip1

// Package users implements a minimal users command.
package users

import (
	"os"

	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the users command. It prints the user names of users
// currently logged in. No flags are supported.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) > 0 {
		return core.UsageError(stdio, "users", "invalid option -- '"+args[0]+"'")
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	stdio.Println(user)
	return core.ExitSuccess
}
