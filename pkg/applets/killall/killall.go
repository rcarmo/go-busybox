// Package killall implements killall using pgrep matching.
package killall

import (
	"github.com/rcarmo/go-busybox/pkg/applets/pgrep"
	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	// killall matches process names (comm) and sends TERM by default.
	return pgrep.Run(stdio, append([]string{"-s", "TERM"}, args...))
}
