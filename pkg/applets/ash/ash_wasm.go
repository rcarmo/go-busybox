//go:build js || wasm || wasip1

package ash

import "github.com/rcarmo/go-busybox/pkg/core"

// Run is a stub that returns an error on WASM platforms where ash is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("ash: not supported in wasm\n")
	return core.ExitFailure
}
