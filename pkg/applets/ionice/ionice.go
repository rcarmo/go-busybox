//go:build !js && !wasm && !wasip1

// Package ionice implements a minimal ionice command.
package ionice

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/rcarmo/go-busybox/pkg/core"
)

const (
	ioprioClassIdle = 3
	ioprioClassBest = 2
	ioprioClassRT   = 1
)

// Run executes the ionice command with the given arguments.
//
// Supported flags:
//
//	-c CLASS    Set I/O scheduling class (1=RT, 2=best-effort, 3=idle)
//	-n LEVEL    Set I/O scheduling priority level (0-7)
//
// Remaining arguments form the command to execute with the given
// I/O scheduling parameters.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		stdio.Println("none: prio 0")
		return core.ExitSuccess
	}
	class := ""
	level := 0
	i := 0
	for i < len(args) {
		switch args[i] {
		case "-c":
			if i+1 >= len(args) {
				return core.UsageError(stdio, "ionice", "missing class")
			}
			class = args[i+1]
			i += 2
		case "-n":
			if i+1 >= len(args) {
				return core.UsageError(stdio, "ionice", "missing level")
			}
			val, err := strconv.Atoi(args[i+1])
			if err != nil {
				return core.UsageError(stdio, "ionice", "invalid level")
			}
			level = val
			i += 2
		default:
			if strings.HasPrefix(args[i], "-") {
				return core.UsageError(stdio, "ionice", "invalid option -- '"+strings.TrimPrefix(args[i], "-")+"'")
			}
			goto run
		}
	}
run:
	if class == "" {
		stdio.Errorf("ionice: missing class\n")
		return core.ExitFailure
	}
	var classVal int
	switch class {
	case "1":
		classVal = ioprioClassRT
	case "2":
		classVal = ioprioClassBest
	case "3":
		classVal = ioprioClassIdle
	default:
		return core.UsageError(stdio, "ionice", "invalid class")
	}
	if level < 0 || level > 7 {
		return core.UsageError(stdio, "ionice", "invalid level")
	}
	if len(args[i:]) < 1 {
		return core.UsageError(stdio, "ionice", "missing command")
	}
	cmd := exec.Command(args[i], args[i+1:]...) // #nosec G204 -- ionice runs user-provided command
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
