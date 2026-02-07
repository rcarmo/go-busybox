//go:build js || wasm || wasip1

package startstopdaemon

import "github.com/rcarmo/go-busybox/pkg/core"

func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("start-stop-daemon: not supported in wasm\n")
	return core.ExitFailure
}
