//go:build !js && !wasm && !wasip1

// Package startstopdaemon implements a minimal start-stop-daemon command.
package startstopdaemon

import (
	"os"
	"os/exec"
	"strconv"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "start-stop-daemon", "missing action")
	}
	if args[0] != "--start" {
		return core.UsageError(stdio, "start-stop-daemon", "only --start supported")
	}
	if len(args) < 3 || args[1] != "--exec" {
		return core.UsageError(stdio, "start-stop-daemon", "missing --exec")
	}
	binary := args[2]
	cmdArgs := []string{}
	i := 3
	for i < len(args) {
		if args[i] == "--" {
			cmdArgs = append(cmdArgs, args[i+1:]...)
			break
		}
		if args[i] == "--pidfile" && i+1 < len(args) {
			pidfile := args[i+1]
			cmdArgs = append(cmdArgs, "--pidfile", pidfile)
			i += 2
			continue
		}
		i++
	}
	cmd := exec.Command(binary, cmdArgs...)
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
	cmd.Stdin = stdio.In
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		stdio.Errorf("start-stop-daemon: %v\n", err)
		return core.ExitFailure
	}
	stdio.Println(strconv.Itoa(cmd.Process.Pid))
	return core.ExitSuccess
}
