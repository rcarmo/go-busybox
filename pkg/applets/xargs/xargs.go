// Package xargs implements a minimal xargs command.
package xargs

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/rcarmo/go-busybox/pkg/core"
)

func Run(stdio *core.Stdio, args []string) int {
	zeroTerm := false
	trace := false
	eofStr := "" // -E
	eofEnabled := false
	maxSize := 0 // -s
	noRun := false
	maxArgs := 0 // -n

	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "-" && args[0] != "--" {
		arg := args[0]
		args = args[1:]
		j := 1
		for j < len(arg) {
			switch arg[j] {
			case '0':
				zeroTerm = true
				j++
			case 't':
				trace = true
				j++
			case 'r':
				noRun = true
				j++
			case 'E':
				val := arg[j+1:]
				if val == "" {
					if len(args) == 0 {
						return core.UsageError(stdio, "xargs", "missing argument for -E")
					}
					val = args[0]
					args = args[1:]
				}
				eofStr = val
				eofEnabled = true
				j = len(arg)
			case 'e':
				val := arg[j+1:]
				if val != "" {
					eofStr = val
					eofEnabled = true
				}
				j = len(arg)
			case 's':
				val := arg[j+1:]
				if val == "" {
					if len(args) == 0 {
						return core.UsageError(stdio, "xargs", "missing argument for -s")
					}
					val = args[0]
					args = args[1:]
				}
				n, err := strconv.Atoi(val)
				if err != nil || n <= 0 {
					return core.UsageError(stdio, "xargs", "invalid argument for -s")
				}
				maxSize = n
				j = len(arg)
			case 'n':
				val := arg[j+1:]
				if val == "" {
					if len(args) == 0 {
						return core.UsageError(stdio, "xargs", "missing argument for -n")
					}
					val = args[0]
					args = args[1:]
				}
				n, err := strconv.Atoi(val)
				if err != nil || n <= 0 {
					return core.UsageError(stdio, "xargs", "invalid argument for -n")
				}
				maxArgs = n
				j = len(arg)
			default:
				return core.UsageError(stdio, "xargs", "invalid option -- '"+string(arg[j])+"'")
			}
		}
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}

	cmdName := "echo"
	var cmdBase []string
	if len(args) > 0 {
		cmdName = args[0]
		cmdBase = args[1:]
	}

	words, err := readWords(stdio.In, zeroTerm, eofEnabled, eofStr)
	if err != nil {
		stdio.Errorf("xargs: %v\n", err)
		return core.ExitFailure
	}

	if noRun && len(words) == 0 {
		return core.ExitSuccess
	}

	if maxSize > 0 {
		return runBatched(stdio, cmdName, cmdBase, words, maxSize, trace)
	}

	if maxArgs > 0 {
		return runBatchedByArgs(stdio, cmdName, cmdBase, words, maxArgs, trace)
	}

	cmdArgs := append(cmdBase, words...)
	return runOne(stdio, cmdName, cmdArgs, trace)
}

func runBatchedByArgs(stdio *core.Stdio, cmdName string, cmdBase []string, words []string, maxArgs int, trace bool) int {
	exitCode := core.ExitSuccess
	for i := 0; i < len(words); i += maxArgs {
		end := i + maxArgs
		if end > len(words) {
			end = len(words)
		}
		cmdArgs := append(append([]string{}, cmdBase...), words[i:end]...)
		rc := runOne(stdio, cmdName, cmdArgs, trace)
		if rc != 0 {
			exitCode = rc
		}
	}
	return exitCode
}

func runBatched(stdio *core.Stdio, cmdName string, cmdBase []string, words []string, maxSize int, trace bool) int {
	exitCode := core.ExitSuccess
	baseLen := len(cmdName)
	for _, b := range cmdBase {
		baseLen += 1 + len(b)
	}

	var batch []string
	currentLen := baseLen
	for _, w := range words {
		newLen := currentLen + 1 + len(w)
		if len(batch) > 0 && newLen+1 > maxSize {
			cmdArgs := append(append([]string{}, cmdBase...), batch...)
			rc := runOne(stdio, cmdName, cmdArgs, trace)
			if rc != 0 {
				exitCode = rc
			}
			batch = nil
			currentLen = baseLen
		}
		batch = append(batch, w)
		currentLen += 1 + len(w)
	}
	if len(batch) > 0 {
		cmdArgs := append(append([]string{}, cmdBase...), batch...)
		rc := runOne(stdio, cmdName, cmdArgs, trace)
		if rc != 0 {
			exitCode = rc
		}
	}
	return exitCode
}

func runOne(stdio *core.Stdio, cmdName string, cmdArgs []string, trace bool) int {
	fullArgs := append([]string{cmdName}, cmdArgs...)
	if trace {
		stdio.Errorf("%s\n", strings.Join(fullArgs, " "))
	}
	cmd := exec.Command(cmdName, cmdArgs...) // #nosec G204
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
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

func readWords(r io.Reader, zeroTerm bool, eofEnabled bool, eofStr string) ([]string, error) {
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
		line := reader.Text()
		if eofEnabled && line == eofStr {
			break
		}
		for _, w := range strings.Fields(line) {
			if eofEnabled && w == eofStr {
				return words, nil
			}
			words = append(words, w)
		}
	}
	if err := reader.Err(); err != nil {
		return nil, err
	}
	return words, nil
}
