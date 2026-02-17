//go:build !js && !wasm && !wasip1

// Package nohup implements a minimal nohup command.
package nohup

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/rcarmo/go-busybox/pkg/core"
	corefs "github.com/rcarmo/go-busybox/pkg/core/fs"
)

// Run executes the nohup command with the given arguments.
//
// nohup runs COMMAND immune to hangups, with output redirected to nohup.out.
// No flags are supported.
func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "nohup", "missing command")
	}
	out, err := corefs.OpenFile("nohup.out", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		stdio.Errorf("nohup: %v\n", err)
		return core.ExitFailure
	}
	defer out.Close()
	cmd := exec.Command(args[0], args[1:]...) // #nosec G204 -- nohup runs user-provided command
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Stdin = stdio.In
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		stdio.Errorf("nohup: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
