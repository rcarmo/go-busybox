// Package pkill implements pkill using pgrep matching.
package pkill

import (
	"github.com/rcarmo/go-busybox/pkg/applets/pgrep"
	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the pkill command with the given arguments.
//
// pkill sends a signal (default SIGTERM) to processes matching a pattern.
// It delegates to the pgrep implementation with signal-sending enabled.
//
// Supported flags:
//
//	-f    Match against full command line, not just process name
//	-x    Require exact match of the process name
//	-v    Negate the matching
//	-u USER  Only match processes owned by USER
//	-s SIG   Send signal SIG instead of SIGTERM
func Run(stdio *core.Stdio, args []string) int {
	// Reuse pgrep with -s to send signal for matches.
	return pgrep.Run(stdio, append([]string{"-s", "TERM"}, args...))
}
