//go:build js || wasm || wasip1

package w

import "github.com/rcarmo/go-busybox/pkg/core"

// Run is a stub that returns an error on WASM platforms where w is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("w: not supported in wasm\n")
	return core.ExitFailure
}
