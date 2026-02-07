//go:build js || wasm || wasip1

package nice

import "github.com/rcarmo/go-busybox/pkg/core"

func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("nice: not supported in wasm\n")
	return core.ExitFailure
}
