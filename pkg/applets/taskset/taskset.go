//go:build !js && !wasm && !wasip1

// Package taskset implements a minimal taskset command.
package taskset

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "taskset", "missing mask or command")
	}
	maskStr := strings.TrimPrefix(args[0], "0x")
	mask, err := strconv.ParseUint(maskStr, 16, 64)
	if err != nil {
		return core.UsageError(stdio, "taskset", "invalid mask")
	}
	var set unix.CPUSet
	set.Zero()
	for cpu := 0; cpu < 64; cpu++ {
		if mask&(1<<uint(cpu)) != 0 {
			set.Set(cpu)
		}
	}
	cmd := exec.Command(args[1], args[2:]...)
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
	cmd.Stdin = stdio.In
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		stdio.Errorf("taskset: %v\n", err)
		return core.ExitFailure
	}
	if err := unix.SchedSetaffinity(cmd.Process.Pid, &set); err != nil {
		stdio.Errorf("taskset: %v\n", err)
		_ = cmd.Process.Kill()
		return core.ExitFailure
	}
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		stdio.Errorf("taskset: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
