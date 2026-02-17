// Package killall implements killall using pgrep matching.
package killall

import (
	"github.com/rcarmo/go-busybox/pkg/applets/pgrep"
	"github.com/rcarmo/go-busybox/pkg/core"
)

// Run executes the killall command with the given arguments.
//
// killall sends a signal (default SIGTERM) to all processes matching
// the given name pattern. It delegates to the pgrep implementation
// with signal-sending enabled.
func Run(stdio *core.Stdio, args []string) int {
	// killall matches process names (comm) and sends TERM by default.
	return pgrep.Run(stdio, append([]string{"-s", "TERM"}, args...))
}
