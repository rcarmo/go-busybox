package grep

import (
	"bufio"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	// parse optional flags (only -n supported)
	showLineNum := false
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if args[i] == "-n" {
			showLineNum = true
			i++
			continue
		}
		return core.UsageError(stdio, "grep", "invalid option")
	}
	if i >= len(args) {
		return core.UsageError(stdio, "grep", "missing pattern or file")
	}
	pattern := args[i]
	i++
	file := "-"
	if i < len(args) {
		file = args[i]
	}
	var scanner *bufio.Scanner
	if file == "-" {
		scanner = bufio.NewScanner(stdio.In)
	} else {
		f, err := fs.Open(file)
		if err != nil {
			stdio.Errorf("grep: %s: %v\n", file, err)
			return core.ExitFailure
		}
		defer f.Close()
		scanner = bufio.NewScanner(f)
	}
	lineNum := 1

	matched := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, pattern) {
			if showLineNum {
				stdio.Printf("%d:%s\n", lineNum, line)
			} else {
				stdio.Printf("%s\n", line)
			}
			matched = true
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		stdio.Errorf("grep: %v\n", err)
		return core.ExitFailure
	}
	if matched {
		return 0
	}
	return 1
}
