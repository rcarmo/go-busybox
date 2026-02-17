//go:build js || wasm || wasip1

// Package renice implements the renice command.
package renice

import "github.com/rcarmo/go-busybox/pkg/core"

// Run is a stub that returns an error on WASM platforms where renice is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("renice: not supported in wasm\n")
	return core.ExitFailure
}
