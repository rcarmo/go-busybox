//go:build !js && !wasm && !wasip1

// Package timeout implements a minimal timeout command.
package timeout

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/rcarmo/go-busybox/pkg/applets/procutil"
	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/timeutil"
)

// Run executes the timeout command with the given arguments.
//
// Supported flags:
//
//	-s SIGNAL   Specify the signal to send on timeout (default SIGTERM)
//	-k DUR      Send SIGKILL after DUR if the process is still running
//	            (accepted for compatibility, not fully implemented)
//
// Duration supports optional suffixes: s (seconds, default), m (minutes),
// h (hours), d (days). Exit code is 143 when the process is killed by
// SIGTERM.
func Run(stdio *core.Stdio, args []string) int {
	sig := syscall.SIGTERM
	if len(args) == 0 {
		return core.UsageError(stdio, "timeout", "missing duration or command")
	}
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		switch args[i] {
		case "-s":
			if i+1 >= len(args) {
				return core.UsageError(stdio, "timeout", "missing signal")
			}
			parsed, err := procutil.ParseSignal(args[i+1])
			if err != nil {
				return core.UsageError(stdio, "timeout", "invalid signal")
			}
			sig = parsed
			i += 2
		case "-k":
			if i+1 >= len(args) {
				return core.UsageError(stdio, "timeout", "missing duration")
			}
			i += 2
		default:
			return core.UsageError(stdio, "timeout", "invalid option -- '"+strings.TrimPrefix(args[i], "-")+"'")
		}
	}
	if len(args)-i < 2 {
		return core.UsageError(stdio, "timeout", "missing duration or command")
	}
	spec, err := timeutil.ParseDuration(args[i])
	if err != nil {
		return core.UsageError(stdio, "timeout", "invalid duration")
	}
	cmd := exec.Command(args[i+1], args[i+2:]...) // #nosec G204 -- timeout runs user-provided command
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
		_ = cmd.Process.Signal(sig)
		if sig == syscall.SIGTERM {
			return 143
		}
		return core.ExitFailure
	}
}
