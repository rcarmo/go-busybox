//go:build js || wasm || wasip1

// Package ss implements a minimal /proc-based ss command.
package ss

import "github.com/rcarmo/go-busybox/pkg/core"

// Run is a stub that returns an error on WASM platforms where ss is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("ss: not supported in wasm\n")
	return core.ExitFailure
}
