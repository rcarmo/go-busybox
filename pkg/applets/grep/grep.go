package grep

import (
	"bufio"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "grep", "missing pattern or file")
	}
	pattern := args[0]
	file := args[1]
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
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, pattern) {
			stdio.Printf("%d:%s\n", lineNum, line)
		}
		lineNum++
	}
	if err := scanner.Err(); err != nil {
		stdio.Errorf("grep: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
