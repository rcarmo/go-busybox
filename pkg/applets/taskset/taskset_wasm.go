//go:build js || wasm || wasip1

package taskset

import "github.com/rcarmo/go-busybox/pkg/core"

func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("taskset: not supported in wasm\n")
	return core.ExitFailure
}
