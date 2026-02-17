//go:build js || wasm || wasip1

package taskset

import "github.com/rcarmo/go-busybox/pkg/core"

// Run is a stub that returns an error on WASM platforms where taskset is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("taskset: not supported in wasm\n")
	return core.ExitFailure
}
