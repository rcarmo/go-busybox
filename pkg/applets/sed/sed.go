package sed

import (
	"bufio"
	"strings"

	"github.com/rcarmo/busybox-wasm/pkg/core"
	"github.com/rcarmo/busybox-wasm/pkg/core/fs"
)

func Run(stdio *core.Stdio, args []string) int {
	if len(args) < 2 {
		return core.UsageError(stdio, "sed", "missing script or file")
	}

	script := args[0]
	// support only s/old/new/ simple form
	if !strings.HasPrefix(script, "s/") {
		return core.UsageError(stdio, "sed", "only simple s/// supported")
	}
	parts := strings.SplitN(script[2:], "/", 2)
	if len(parts) < 2 {
		return core.UsageError(stdio, "sed", "invalid script")
	}
	old := parts[0]
	new := parts[1]
	// strip trailing delimiter if user provided s/foo/bar/ form that left an empty string
	if strings.HasSuffix(new, "/") {
		new = strings.TrimSuffix(new, "/")
	}
	file := args[1]
	var scanner *bufio.Scanner
	if file == "-" {
		scanner = bufio.NewScanner(stdio.In)
	} else {
		f, err := fs.Open(file)
		if err != nil {
			stdio.Errorf("sed: %s: %v\n", file, err)
			return core.ExitFailure
		}
		defer f.Close()
		scanner = bufio.NewScanner(f)
	}
	for scanner.Scan() {
		line := scanner.Text()
		stdio.Println(strings.ReplaceAll(line, old, new))
	}
	if err := scanner.Err(); err != nil {
		stdio.Errorf("sed: %v\n", err)
		return core.ExitFailure
	}
	return core.ExitSuccess
}
