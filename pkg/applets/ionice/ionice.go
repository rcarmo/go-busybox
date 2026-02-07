//go:build !js && !wasm && !wasip1

// Package ionice implements a minimal ionice command.
package ionice

import (
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/unix"

	"github.com/rcarmo/go-busybox/pkg/core"
)

const (
	ioprioClassIdle = 3
	ioprioClassBest = 2
	ioprioClassRT   = 1
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "ionice", "missing class or command")
	}
	class := args[0]
	var classVal int
	switch class {
	case "idle":
		classVal = ioprioClassIdle
	case "best":
		classVal = ioprioClassBest
	case "rt":
		classVal = ioprioClassRT
	default:
		return core.UsageError(stdio, "ionice", "invalid class")
	}
	level, err := strconv.Atoi(args[1])
	if err != nil || level < 0 || level > 7 {
		return core.UsageError(stdio, "ionice", "invalid level")
	}
	if len(args) < 3 {
		return core.UsageError(stdio, "ionice", "missing command")
	}
	cmd := exec.Command(args[2], args[3:]...) // #nosec G204 -- ionice runs user-provided command
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
	cmd.Stdin = stdio.In
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		stdio.Errorf("ionice: %v\n", err)
		return core.ExitFailure
	}
	if err := setIOPrio(cmd.Process.Pid, classVal, level); err != nil {
		stdio.Errorf("ionice: %v\n", err)
		_ = cmd.Process.Kill()
		return core.ExitFailure
	}
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		stdio.Errorf("ionice: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}

func setIOPrio(pid int, class int, level int) error {
	// ioprio_set(2) encoding: (class << 13) | level.
	prio := (class << 13) | level
	_, _, errno := unix.RawSyscall(unix.SYS_IOPRIO_SET, uintptr(1), uintptr(pid), uintptr(prio))
	if errno != 0 {
		return errno
	}
	return nil
}
