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

// Run executes the taskset command with the given arguments.
//
// Usage:
//
//	taskset MASK COMMAND [ARG...]   Run COMMAND with CPU affinity MASK
//	taskset -p PID                  Display current affinity of PID
//
// MASK is a hexadecimal CPU affinity mask (e.g., 0x3 for CPUs 0 and 1).
func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "taskset", "missing mask or command")
	}
	if args[0] == "-p" {
		if len(args) < 2 {
			return core.UsageError(stdio, "taskset", "missing pid")
		}
		pid, err := strconv.Atoi(args[1])
		if err != nil {
			return core.UsageError(stdio, "taskset", "number "+args[1]+" is not in 1..2147483647 range")
		}
		if pid < 1 {
			stdio.Errorf("taskset: failed to get pid %d's affinity\n", pid)
			return core.ExitFailure
		}
		mask, err := getAffinityMask(pid)
		if err != nil {
			stdio.Errorf("taskset: %v\n", err)
			return core.ExitFailure
		}
		stdio.Printf("pid %d's current affinity mask: %x\n", pid, mask)
		return core.ExitSuccess
	}
	maskStr := strings.TrimPrefix(args[0], "0x")
	mask, err := strconv.ParseUint(maskStr, 16, 64)
	if err != nil {
		return core.UsageError(stdio, "taskset", "invalid mask")
	}
	var set unix.CPUSet
	set.Zero()
	for cpu := 0; cpu < 64; cpu++ {
		if mask&(1<<cpu) != 0 {
			set.Set(cpu)
		}
	}
	cmd := exec.Command(args[1], args[2:]...) // #nosec G204 -- taskset executes requested command
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

func getAffinityMask(pid int) (uint64, error) {
	var set unix.CPUSet
	if err := unix.SchedGetaffinity(pid, &set); err != nil {
		return 0, err
	}
	var mask uint64
	for cpu := 0; cpu < 64; cpu++ {
		if set.IsSet(cpu) {
			mask |= 1 << cpu
		}
	}
	return mask, nil
}
