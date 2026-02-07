//go:build !js && !wasm && !wasip1

// Package timeout implements a minimal timeout command.
package timeout

import (
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/timeutil"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "timeout", "missing duration or command")
	}
	spec, err := timeutil.ParseDuration(args[0])
	if err != nil {
		return core.UsageError(stdio, "timeout", "invalid duration")
	}
	cmd := exec.Command(args[1], args[2:]...)
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
	cmd.Stdin = stdio.In
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		stdio.Errorf("timeout: %v\n", err)
		return core.ExitFailure
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode()
			}
			stdio.Errorf("timeout: %v\n", err)
			return core.ExitFailure
		}
		return core.ExitSuccess
	case <-time.After(spec.Duration):
		_ = cmd.Process.Signal(syscall.SIGTERM)
		return core.ExitFailure
	}
}
