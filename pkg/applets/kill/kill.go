// Package kill implements the kill command.
package kill

import (
	"strconv"
	"strings"
	"syscall"

	"github.com/rcarmo/go-busybox/pkg/applets/procutil"
	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the kill command with the given arguments.
//
// Usage:
//
//	kill [-SIGNAL] PID...
//	kill -l
//
// Sends a signal to the specified processes. The default signal is SIGTERM.
// The signal can be specified by name (e.g., -TERM, -9) or number.
// Use -l to list all available signal names and numbers.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "kill", "missing pid")
	}
	if args[0] == "-l" {
		printSignals(stdio)
		return core.ExitSuccess
	}
	sig, err := procutil.ParseSignal(args[0])
	if err == nil {
		args = args[1:]
	} else {
		sig = syscall.SIGTERM
	}
	if len(args) == 0 {
		return core.UsageError(stdio, "kill", "missing pid")
	}
	for _, pidStr := range args {
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			return core.UsageError(stdio, "kill", "invalid pid: "+pidStr)
		}
		if err := syscall.Kill(pid, sig); err != nil {
			stdio.Errorf("kill: %v\n", err)
			return core.ExitFailure
		}
	}
	return core.ExitSuccess
}

func printSignals(stdio *core.Stdio) {
	names := procutil.SignalNames()
	out := make([]string, 0, len(names))
	for sig, name := range names {
		out = append(out, strconv.Itoa(int(sig))+" "+name)
	}
	stdio.Println(strings.Join(out, "\n"))
}
