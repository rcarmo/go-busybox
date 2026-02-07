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
	stdio.Printf("real\t%s\n", timeutil.FormatDuration(spec))
	stdio.Printf("user\t0.00\n")
	stdio.Printf("sys\t0.00\n")
}
