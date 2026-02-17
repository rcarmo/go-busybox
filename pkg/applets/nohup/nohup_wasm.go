//go:build js || wasm || wasip1

package nohup

import "github.com/rcarmo/go-busybox/pkg/core"

// Run is a stub that returns an error on WASM platforms where nohup is unsupported.
func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("nohup: not supported in wasm\n")
	return core.ExitFailure
}
