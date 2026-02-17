// Package pgrep implements pgrep/pkill-style matching.
package pgrep

import (
	"strconv"
	"strings"
	"syscall"

	"github.com/rcarmo/go-busybox/pkg/applets/procutil"
	"github.com/rcarmo/go-busybox/pkg/core"
)

type options struct {
	useArgs bool
	exact   bool
	invert  bool
	user    string
	signal  syscall.Signal
	kill    bool
}

// Run executes the pgrep command with the given arguments.
//
// Supported flags:
//
//	-f          Match against full command line, not just process name
//	-x          Require exact match
//	-v          Negate the matching
//	-u USER     Only match processes owned by USER
//	-l          (accepted for compatibility, not implemented)
//	-s SIGNAL   Send SIGNAL to matched processes (used by pkill/killall)
//
// Prints matching PIDs, one per line. Returns exit code 0 if at least
// one process matched, 1 otherwise.
func Run(stdio *core.Stdio, args []string) int {
	opts := options{signal: syscall.SIGTERM}
	var patterns []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-f":
			opts.useArgs = true
		case "-x":
			opts.exact = true
		case "-v":
			opts.invert = true
		case "-u":
			if i+1 >= len(args) {
				return core.UsageError(stdio, "pgrep", "missing user")
			}
			i++
			opts.user = args[i]
		case "-l":
			// busybox pgrep doesn't support -l; ignore for now
		case "-s":
			if i+1 >= len(args) {
				return core.UsageError(stdio, "pgrep", "missing signal")
			}
			i++
			sig, err := procutil.ParseSignal(args[i])
			if err != nil {
				return core.UsageError(stdio, "pgrep", "invalid signal")
			}
			opts.signal = sig
			opts.kill = true
		default:
			if strings.HasPrefix(arg, "-") {
				return core.UsageError(stdio, "pgrep", "invalid option -- '"+strings.TrimPrefix(arg, "-")+"'")
			}
			patterns = append(patterns, arg)
		}
	}
	if len(patterns) == 0 {
		return core.UsageError(stdio, "pgrep", "missing pattern")
	}
	matches := procutil.MatchProcs(patterns, procutil.MatchOptions{
		UseArgs: opts.useArgs,
		Exact:   opts.exact,
		Invert:  opts.invert,
		User:    opts.user,
	})
	procutil.SortByPID(matches)
	if len(matches) == 0 {
		return core.ExitFailure
	}
	if opts.kill {
		for _, proc := range matches {
			if err := syscall.Kill(proc.PID, opts.signal); err != nil {
				stdio.Errorf("pkill: %v\n", err)
				return core.ExitFailure
			}
		}
		return core.ExitSuccess
	}
	var out []string
	for _, proc := range matches {
		out = append(out, strconv.Itoa(proc.PID))
	}
	stdio.Println(strings.Join(out, "\n"))
	return core.ExitSuccess
}
