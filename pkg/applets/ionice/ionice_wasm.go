//go:build js || wasm || wasip1

package ionice

import "github.com/rcarmo/go-busybox/pkg/core"

// Run is a stub that returns an error on WASM platforms where ionice is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("ionice: not supported in wasm\n")
	return core.ExitFailure
}
