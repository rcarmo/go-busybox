//go:build !js && !wasm && !wasip1

// Package watch implements a minimal watch command.
package watch

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	interval := 2 * time.Second
	if len(args) == 0 {
		return core.UsageError(stdio, "watch", "missing command")
	}
	if args[0] == "-n" {
		if len(args) < 3 {
			return core.UsageError(stdio, "watch", "missing interval")
		}
		value, err := time.ParseDuration(args[1] + "s")
		if err != nil {
			return core.UsageError(stdio, "watch", "invalid interval")
		}
		interval = value
		args = args[2:]
	}
	if len(args) == 0 {
		return core.UsageError(stdio, "watch", "missing command")
	}
	cmdStr := strings.Join(args, " ")
	cmd := exec.Command("sh", "-c", cmdStr) // #nosec G204 -- watch executes user-provided shell command
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
	cmd.Stdin = stdio.In
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		stdio.Errorf("watch: %v\n", err)
		return core.ExitFailure
	}
	if interval > 0 {
		time.Sleep(interval)
	}
	return core.ExitSuccess
}
