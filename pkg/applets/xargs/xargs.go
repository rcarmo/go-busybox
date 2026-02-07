// Package xargs implements a minimal xargs command.
package xargs

import (
	"bufio"
	"os"
	"os/exec"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "xargs", "missing command")
	}
	reader := bufio.NewScanner(stdio.In)
	var words []string
	for reader.Scan() {
		words = append(words, strings.Fields(reader.Text())...)
	}
	if err := reader.Err(); err != nil {
		stdio.Errorf("xargs: %v\n", err)
		return core.ExitFailure
	}
	cmdArgs := append(args, words...)
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...) // #nosec G204 -- xargs runs user-provided command
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
	cmd.Stdin = stdio.In
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		stdio.Errorf("xargs: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
