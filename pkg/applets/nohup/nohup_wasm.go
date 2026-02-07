//go:build js || wasm || wasip1

package nohup

import "github.com/rcarmo/go-busybox/pkg/core"

func Run(stdio *core.Stdio, args []string) int {
	stdio.Errorf("nohup: not supported in wasm\n")
	return core.ExitFailure
}
