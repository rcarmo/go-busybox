// Package pkill implements pkill using pgrep matching.
package pkill

import (
	"github.com/rcarmo/go-busybox/pkg/applets/pgrep"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	// Reuse pgrep with -s to send signal for matches.
	return pgrep.Run(stdio, append([]string{"-s", "TERM"}, args...))
}
