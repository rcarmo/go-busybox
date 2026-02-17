//go:build js || wasm || wasip1

package watch

import "github.com/rcarmo/go-busybox/pkg/core"

// Run is a stub that returns an error on WASM platforms where watch is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("watch: not supported in wasm\n")
	return core.ExitFailure
}
