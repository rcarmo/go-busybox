//go:build js || wasm || wasip1

package startstopdaemon

import "github.com/rcarmo/go-busybox/pkg/core"

// Run is a stub that returns an error on WASM platforms where startstopdaemon is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("start-stop-daemon: not supported in wasm\n")
	return core.ExitFailure
}
