//go:build !js && !wasm && !wasip1

// Package time implements a minimal time command.
package time

import (
	"os"
	"os/exec"
	"time"

	"github.com/rcarmo/go-busybox/pkg/core"
	"github.com/rcarmo/go-busybox/pkg/core/timeutil"
)

// Run executes the time command with the given arguments.
//
// time runs COMMAND and prints a summary of real elapsed time to stderr.
// No flags are supported. User and system CPU times are reported as 0.00
// since per-process accounting is not available.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "time", "missing command")
	}
	cmd := exec.Command(args[0], args[1:]...) // #nosec G204 -- time runs user-provided command
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
	cmd.Stdin = stdio.In
	cmd.Env = os.Environ()
	start := time.Now()
	if err := cmd.Start(); err != nil {
		stdio.Errorf("time: %v\n", err)
		return core.ExitFailure
	}
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			printElapsed(stdio, time.Since(start))
			return exitErr.ExitCode()
		}
		stdio.Errorf("time: %v\n", err)
		return core.ExitFailure
	}
	printElapsed(stdio, time.Since(start))
	return core.ExitSuccess
}

func printElapsed(stdio *core.Stdio, elapsed time.Duration) {
	sec := elapsed.Seconds()
	spec := timeutil.DurationSpec{Duration: elapsed, Unit: "s", Value: sec}
	stdio.Errorf("real\t%s\n", timeutil.FormatDuration(spec))
	stdio.Errorf("user\t0.00\n")
	stdio.Errorf("sys\t0.00\n")
}
