//go:build js || wasm || wasip1

package watch

import "github.com/rcarmo/go-busybox/pkg/core"

func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("watch: not supported in wasm\n")
	return core.ExitFailure
}
