//go:build !js && !wasm && !wasip1

// Package nice implements a minimal nice command.
package nice

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	priority := 10
	if len(args) > 0 && args[0] == "-n" {
		if len(args) < 3 {
			return core.UsageError(stdio, "nice", "missing priority")
		}
		value, err := strconv.Atoi(args[1])
		if err != nil {
			return core.UsageError(stdio, "nice", "invalid number '"+args[1]+"'")
		}
		priority = value
		args = args[2:]
	}
	if len(args) == 0 {
		return core.UsageError(stdio, "nice", "missing command")
	}
	cmd := exec.Command(args[0], args[1:]...) // #nosec G204 -- nice runs user-provided command
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
	cmd.Stdin = stdio.In
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		stdio.Errorf("nice: %v\n", err)
		return core.ExitFailure
	}
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, priority)
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		stdio.Errorf("nice: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
