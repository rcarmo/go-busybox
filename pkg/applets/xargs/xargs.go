// Package xargs implements a minimal xargs command.
package xargs

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) == 0 {
		return core.UsageError(stdio, "xargs", "missing command")
	}
	zeroTerm := false
	trace := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-0":
			zeroTerm = true
			args = args[1:]
		case "-t":
			trace = true
			args = args[1:]
		default:
			return core.UsageError(stdio, "xargs", "invalid option -- '"+strings.TrimPrefix(args[0], "-")+"'")
		}
	}
	if len(args) == 0 {
		return core.UsageError(stdio, "xargs", "missing command")
	}
	words, err := readWords(stdio.In, zeroTerm)
	if err != nil {
		stdio.Errorf("xargs: %v\n", err)
		return core.ExitFailure
	}
	cmdArgs := append(args, words...)
	if trace {
		stdio.Errorf("%s\n", strings.Join(cmdArgs, " "))
	}
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

func readWords(r io.Reader, zeroTerm bool) ([]string, error) {
	if zeroTerm {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		parts := bytes.Split(data, []byte{0})
		words := make([]string, 0, len(parts))
		for _, part := range parts {
			if len(part) == 0 {
				continue
			}
			words = append(words, string(part))
		}
		return words, nil
	}
	reader := bufio.NewScanner(r)
	var words []string
	for reader.Scan() {
		words = append(words, strings.Fields(reader.Text())...)
	}
	if err := reader.Err(); err != nil {
		return nil, err
	}
	return words, nil
}
