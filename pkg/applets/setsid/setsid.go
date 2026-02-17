//go:build !js && !wasm && !wasip1

// Package setsid implements a minimal setsid command.
package setsid

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the setsid command with the given arguments.
//
// setsid runs COMMAND in a new session, detaching it from the
// controlling terminal. No flags are supported.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "setsid", "missing command")
	}
	cmd := exec.Command(args[0], args[1:]...) // #nosec G204 -- setsid runs user-provided command
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
	cmd.Stdin = stdio.In
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		stdio.Errorf("setsid: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
